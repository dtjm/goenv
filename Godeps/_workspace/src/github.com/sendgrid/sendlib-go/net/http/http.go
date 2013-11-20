package http

import (
	"io"
	"net/http"
	"net/url"
	"strings"
)

type headerLine map[string]string

func parseHeaderLine(line string) headerLine {
	parsed := make(map[string]string)

	parts := strings.Split(line, ";")
	for _, pair := range parts {
		keyVal := strings.Split(pair, "=")
		if len(keyVal) == 2 {
			parsed[keyVal[0]] = keyVal[1]
		}
	}

	return parsed
}

func (hl headerLine) get(k string) string {
	if v, ok := hl[k]; ok {
		return v
	}
	return ""
}

type Client interface {
	Do(req *http.Request) (resp *http.Response, err error)
	Get(url string) (resp *http.Response, err error)
	Head(url string) (resp *http.Response, err error)
	Post(url string, bodyType string, body io.Reader) (resp *http.Response, err error)
	PostForm(url string, data url.Values) (resp *http.Response, err error)
}
