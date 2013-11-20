package http

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/sendgrid/sendlib-go/cache"
	"io"
	"net/http"
	"net/url"
)

// Caching client supports HTTP caching via the Cache-Control, Modified, and
// Expires, and ETag headers
type CachingClient struct {
	cache  cache.ByteCacher
	client Client
}

func (c *CachingClient) Do(req *http.Request) (resp *http.Response, err error) {
	key := makeRequestKey(req)
	if rspData, exists, err := c.cache.Get(key); exists && err == nil && rspData != nil {
		buf := bytes.NewBuffer(rspData)
		bufReader := bufio.NewReader(buf)
		return http.ReadResponse(bufReader, req)
	}

	rsp, err := c.client.Do(req)
	if err != nil {
		return rsp, err
	}

	cacheErr := c.cacheResponse(rsp)
	if cacheErr != nil {
		return nil, cacheErr
	}

	return rsp, err
}

// TODO: Implement this!
func (c *CachingClient) cacheResponse(resp *http.Response) error {
	return nil
}

func (c *CachingClient) Get(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *CachingClient) Head(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *CachingClient) Post(url string, bodyType string, body io.Reader) (resp *http.Response, err error) {
	return c.client.Post(url, bodyType, body)
}

func (c *CachingClient) PostForm(url string, data url.Values) (resp *http.Response, err error) {
	return c.client.PostForm(url, data)
}

func makeRequestKey(r *http.Request) string {
	return fmt.Sprintf("%s %s Accept: %s", r.Method, r.URL.String(), r.Header.Get("Accept"))
}
