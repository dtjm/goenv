// Package sendlib/config implements a config parser compatible with existing
// config files used within SendGrid. E.g., `/etc/default/sendgrid.conf`
package config

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

const defaultConfigFile = "/etc/default/sendgrid.conf"

type Configger interface {
	// Takes a key and a default value. Returns the associated value if found,
	// or the default if not
	GetString(key, theDefault string) string
}

var l *log.Logger = log.New(os.Stderr, "[sendlib/config]", log.Lshortfile)

type defaultConfig struct {
	// The cached and parsed config information
	data map[string]string
}

// Given a file path, returns a Config object which can provide config values
// out of that file.
// TODO: Check mtime of the file and reload the file if necessary
func NewFromFile(path string) (cfg Configger, err error) {
	file, err := os.Open(path)

	if err != nil {
		err = fmt.Errorf("Unable to open config file: '%s' (%s)", path, err.Error)
		return nil, err
	}
	defer file.Close()

	config := new(defaultConfig)
	config.data, err = parseData(file)

	if err != nil {
		return nil, err
	}

	return config, nil
}

// Fetches a value from the config object as a string. If the value does not
// exist, then returns the second parameter.
func (c *defaultConfig) GetString(key, theDefault string) string {
	if val, exists := c.data[key]; exists {
		return val
	} else {
		return theDefault
	}
}

// Reads config data from an io.Reader returns the parsed config as a map
func parseData(r io.Reader) (map[string]string, error) {
	data := make(map[string]string, 256)
	scanner := bufio.NewScanner(r)
	prefix := ""

	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()

		if len(line) == 0 {
			continue
		}

		// Split out comment parts of this line
		commentParts := strings.Split(line, "#")

		// Trim whitespace of the data portion of the line
		linePayload := strings.TrimSpace(commentParts[0])

		if len(linePayload) == 0 {
			continue
		}

		// Check if this is a comment
		if linePayload[0] == '[' && linePayload[len(linePayload)-1] == ']' {
			prefix = linePayload[1:len(linePayload)-1] + "."
			continue
		}

		// Split data into key and value parts
		keyValuePair := strings.Split(linePayload, "=")

		// Check if there are 2 parts
		if len(keyValuePair) != 2 {
			return nil, fmt.Errorf("Missing config value: line %d: '%s'", lineNum, line)
		}

		// Trim whitespace from key and value
		key := strings.TrimSpace(keyValuePair[0])
		value := strings.TrimSpace(keyValuePair[1])

		if _, exists := data[prefix+key]; exists {
			return nil, fmt.Errorf("Duplicate config key: line %d: '%s'", lineNum, line)
		}

		if prefix == "" {
			return nil, fmt.Errorf("Key declared without [section] block: line %d: '%s'", lineNum, line)
		}
		data[prefix+key] = value

		lineNum++
	}

	return data, nil
}
