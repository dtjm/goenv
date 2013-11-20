package http

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
)

type pendingResponse struct {
	*http.Response
	error
}

type PiggybackingClient struct {
	client          Client
	waitgroups      map[string]*sync.WaitGroup
	pendingRequests map[string][]chan *pendingResponse
}

func NewPiggybackingClient(client Client) *PiggybackingClient {
	if client == nil {
		client = &http.Client{}
	}
	return &PiggybackingClient{
		client:          client,
		waitgroups:      make(map[string]*sync.WaitGroup),
		pendingRequests: make(map[string][]chan *pendingResponse),
	}
}

func (c *PiggybackingClient) Do(req *http.Request) (resp *http.Response, err error) {
	key := makeRequestKey(req)
	idempotent := req.Method == "GET" || req.Method == "PUT" || req.Method == "DELETE"

	if pendingRequests, exists := c.pendingRequests[key]; exists && idempotent {
		c := make(chan *pendingResponse)
		pendingRequests = append(pendingRequests, c)
		pr := <-c
		return pr.Response, pr.error
	}

	if idempotent {
		c.pendingRequests[key] = make([]chan *pendingResponse, 0)
	}
	resp, err = c.client.Do(req)

	if !idempotent {
		return resp, err
	}

	body, err := ioutil.ReadAll(resp.Body)

	// Dequeue all pending requests
	for _, c := range c.pendingRequests[key] {
		go func() {
			respCopy := resp
			if respCopy != nil {
				respCopy.Body = ioutil.NopCloser(bytes.NewBuffer(body))
			}
			c <- &pendingResponse{respCopy, err}
		}()
	}

	return resp, err
}

func (c *PiggybackingClient) Get(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}
func (c *PiggybackingClient) Head(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *PiggybackingClient) Post(url string, bodyType string, body io.Reader) (resp *http.Response, err error) {
	return c.client.Post(url, bodyType, body)
}

func (c *PiggybackingClient) PostForm(url string, data url.Values) (resp *http.Response, err error) {
	return c.client.PostForm(url, data)
}
