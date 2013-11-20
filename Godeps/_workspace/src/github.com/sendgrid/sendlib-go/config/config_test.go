package config

import (
	"bytes"
	"fmt"
	"log"
	"testing"
)

func ExampleConfigger() {
	cfg, err := NewFromFile("/etc/default/sendgrid.conf")
	if err != nil {
        log.Fatalf("Unable to load config: %s", err.Error())
	}

	// Reference [section] blocks within the config file by prefixing the key with
	// `section.`
    value := cfg.GetString("section.MY_SETTING", "DEFAULT")

    fmt.Printf("MY_SETTING=%s", value)
}

func TestParseData(t *testing.T) {
	configData := bytes.NewBufferString(`
    # A comment
    [section]
    A=a
    B = b
    C=c # Commented
    E=

    [section.sub]
    D=d
    `)

	data, _ := parseData(configData)

	expected := map[string]string{
		"section.A":     "a",
		"section.B":     "b",
		"section.C":     "c",
		"section.sub.D": "d",
	}

	for key, val := range expected {
		if data[key] != val {
			t.Errorf("Expected %s for %s, got '%s'", val, key, data[key])
		}
	}
}

func TestParseDataNoSection(t *testing.T) {
	configData := bytes.NewBufferString(`A=a`)
    _, err := parseData(configData)
    if err == nil {
        t.Errorf("Config key without section should return error")
    }
}

func TestGetString(t *testing.T) {
	configData := bytes.NewBufferString(`
    [section]
    A=a`)

	cfg := defaultConfig{}
	cfg.data, _ = parseData(configData)
	val := cfg.GetString("section.A", "")
	if val != "a" {
		t.Errorf("Expected 'a' for 'section.A', got '%s'", val)
	}

	// Get default value
	val = cfg.GetString("section.B", "default")
	if val != "default" {
		t.Errorf("Expected default")
	}
}

func TestInvalidLine(t *testing.T) {
	configData := bytes.NewBufferString(`
    [section]
    A`)

	_, err := parseData(configData)
	if err == nil {
		t.Errorf("Expected parse error")
	}
}

func TestDuplicateKey(t *testing.T) {
	configData := bytes.NewBufferString(`
    [section]
    A=a
    A=b`)
	_, err := parseData(configData)
	if err == nil {
		t.Errorf("Expected parse error")
	}

}

func TestNewFromFile(t *testing.T) {
    // Empty file (this is OK)
	cfg, err := NewFromFile("/dev/null")
	if err != nil || cfg == nil {
		t.Errorf("Didn't return any config")
	}

    // Non-existent file
	cfg, err = NewFromFile("/xxx")
	if err == nil {
		t.Errorf("Should fail to create config with non-existent file")
	}

    // File with invalid config data
    cfg, err = NewFromFile("/etc/hosts")
    if err == nil {
        t.Errorf("Should fail to create config with invalid data")
    }


}
