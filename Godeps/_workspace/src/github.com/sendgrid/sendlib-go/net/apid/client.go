// TODO: Refactor call and unmarshal into a single method
package apid

import (
	"encoding/json"
	"fmt"
	"github.com/sendgrid/sendlib-go/cache"
	"github.com/sendgrid/sendlib-go/log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type functionMetadata struct {
	Function  string            `json:"function"`
	Path      string            `json:"path"`
	ResultKey string            `json:"return"`
	Params    map[string]string `json:"params"`
	Cachable  int               `json:"cachable"`
}

type Client struct {
	addr            string
	functions       map[string]functionMetadata
	cache           cache.ByteCacher
	pendingRequests map[string]*sync.WaitGroup
	syslog          log.Syslogger
}

type Args map[string]interface{}

func (a *Args) URLValues() url.Values {
	formData := url.Values{}

	for k, v := range *a {
		switch v.(type) {
		case int:
			formData.Set(k, fmt.Sprintf("%d", v))
		case string:
			formData.Set(k, v.(string))
		}
	}

	return formData
}

func (c *Client) GetParseHostSettings(host string) (*ParseHostSettings, *Error) {
	var phs ParseHostSettings
	out := make(map[string]interface{})
	apidErr := c.callAndUnmarshal("getParseHostSettings",
		&Args{"host": host},
		&out)

	if apidErr != nil {
		return nil, apidErr
	}

	if out["user_id"] != nil {
		phs.UserID = int(out["user_id"].(float64))
		phs.URL = out["url"].(string)
		phs.SpamCheckOutgoing = out["spam_check_outgoing"].(float64) == 1
		phs.SendRaw = out["send_raw"].(float64) == 1
	}

	return &phs, nil
}

func (c *Client) GetUser(userId int) (*User, *Error) {
	var user User
	apidErr := c.callAndUnmarshal("getUser", &Args{"userid": userId}, &user)
	return &user, apidErr
}

func makeRequestKey(function string, args *Args) string {
	return fmt.Sprintf("%s(%v)", function, *args)
}

// Instantiates a new Client pointing at the given addr (host[:port])
func NewClient(addr string, cache cache.ByteCacher, syslogger log.Syslogger) (*Client, error) {
	c := Client{addr: addr, syslog: syslogger}

	err := c.loadFunctions(5)

	c.cache = cache
	c.pendingRequests = make(map[string]*sync.WaitGroup)
	return &c, err
}

func (c *Client) callAndUnmarshal(function string, args *Args, out interface{}) *Error {
	rawJSON, apidErr := c.call(function, args)
	if apidErr != nil {
		return apidErr
	}

	err := json.Unmarshal(*rawJSON, out)
	if err != nil {
		return &Error{
			Code:    598,
			Message: fmt.Sprintf("Error decoding JSON: '%s'", err),
		}
	}

	return nil
}

func (c *Client) call(function string, args *Args) (*json.RawMessage, *Error) {
	functionMeta, exists := c.functions[function]
	if !exists {
		return nil, &Error{
			Code:    597,
			Message: fmt.Sprintf("Function '%s' does not exist", function)}
	}

	formData := args.URLValues()
	key := makeRequestKey(function, args)

	// Check if we already have a pending request (only if it's a cachable response)
	var wg *sync.WaitGroup
	if wg, exists := c.pendingRequests[key]; exists && functionMeta.Cachable > 0 {
		//c.syslog.Debugf("Request pending for %s, waiting", key)
		wg.Wait()
	}

	// Check the cache. If we just finished a pending request, this should be available
	if cacheData, exists, err := c.cache.Get(key); exists && cacheData != nil && err != nil {
		return (*json.RawMessage)(&cacheData), nil
	}

	// If this request is cachable, create a WaitGroup so we can piggyback other requests
	if functionMeta.Cachable > 0 {
		wg = new(sync.WaitGroup)
		c.pendingRequests[key] = wg
		wg.Add(1)
	}

	url := fmt.Sprintf("http://%s%s", c.addr, functionMeta.Path)
	rsp, err := http.PostForm(url, formData)

	if err != nil {
		return nil, &Error{
			Code: 599,
			Message: fmt.Sprintf("Error in http request: %s url=%s data=%s",
				err, url, formData.Encode())}
	}

	decoder := json.NewDecoder(rsp.Body)
	if rsp.StatusCode != 200 {
		var apidErr Error
		decoder.Decode(apidErr)
		apidErr.Code = rsp.StatusCode
		return nil, &apidErr
	}

	var wrapper map[string]*json.RawMessage
	err = decoder.Decode(&wrapper)
	if err != nil {
		return nil, &Error{
			Code: 598,
			Message: fmt.Sprintf("Error decoding JSON: '%s' url=%s data=%s",
				err, url, formData.Encode())}
	}
	//c.syslog.Debugf("wrapper: %v", wrapper)

	result, exists := wrapper[functionMeta.ResultKey]
	if !exists {
		return nil, &Error{
			Code: 596,
			Message: fmt.Sprintf("No result found in JSON property '%s' url=%s data=%s",
				functionMeta.ResultKey, url, formData.Encode())}
	}
	if functionMeta.Cachable > 0 {
		c.cache.Set(key, []byte(*result),
			time.Duration(functionMeta.Cachable)*time.Second)

		// Signal the wait group after caching so the piggybacked requests can
		// use the cache
		wg.Done()
	}

	return result, nil
}

func (c *Client) loadFunctions(retries int) error {
	// Get functions.json
	functionsURL := fmt.Sprintf("http://%s/api/functions.json", c.addr)
	var rsp *http.Response
	var err error
	retryDelay := 3 * time.Second

	for retries > 0 {
		rsp, err = http.Get(functionsURL)

		if err == nil && rsp.StatusCode == 200 {
			break
		}

		c.syslog.Debugf("Error while fetching functions.json: %s", err)
		c.syslog.Debugf("Retrying after %v", retryDelay)
		time.Sleep(retryDelay)
	}

	if err != nil {
		return fmt.Errorf("Failed to load functions: %s", err)
	}

	// Parse functions.json
	decoder := json.NewDecoder(rsp.Body)
	functionsWrapper := make(map[string]map[string]functionMetadata)
	err = decoder.Decode(&functionsWrapper)

	if err != nil {
		return fmt.Errorf("Error decoding functions.json: %s", err)
	}

	var exists bool
	c.functions, exists = functionsWrapper["functions"]
	if !exists {
		return fmt.Errorf("No functions found in JSON")
	}

	c.syslog.Debugf("Loaded %d functions", len(c.functions))
	return nil
}
