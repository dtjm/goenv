package apid

import (
	"github.com/sendgrid/sendlib-go/cache"
	"github.com/sendgrid/sendlib-go/log"
	"sync"
	"testing"
)

var cacher *cache.MemoryCache = cache.NewMemoryCache(1<<10, 1<<10)
var logBuffer *log.StringWriter = log.NewStringWriter()
var syslogger *log.WriterSyslog = log.NewWriterSyslog(logBuffer)

func TestNewClient(t *testing.T) {
	t.Parallel()
	srv := NewMockServer()
	mockUser := User{Id: 180, Email: "tim@sendgrid.net"}
	srv.MockFunction("getUser", 0, func() (interface{}, *Error) {
		return mockUser, nil
	})

	c, err := NewClient(srv.URL, cacher, syslogger)

	if !logBuffer.Match("Loaded \\d+ functions") {
		t.Errorf("Expected log message not logged")
	}
	if c == nil || err != nil {
		t.Errorf("Failed to instantiate client: %s", err)
	}
	user, apidErr := c.GetUser(180)
	if user == nil {
		t.Errorf("Failed to get user 180: %s", apidErr)
	} else if *user != mockUser {
		t.Errorf("Didn't get the user we expected")
	}

	call := srv.NextCall()
	t.Logf("Got user: %v", user)
	t.Logf("Got call: %v", call)
}

func TestCall(t *testing.T) {
	t.Parallel()
	srv := NewMockServer()
	srv.MockFunction("mockfunc", 1, /*cache TTL*/
		func() (interface{}, *Error) {
			return map[string]string{
				"foo": "foo"}, nil
		})
	c, err := NewClient(srv.URL, cacher, syslogger)
	if err != nil {
		t.Fatalf("Error instantiating client: %s", err)
	}

	n := 1
	wg := sync.WaitGroup{}
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			rsp, apidErr := c.call("mockfunc", &Args{})

			if apidErr != nil {
				t.Fatalf("Couldn't call function: %s", apidErr)
			}
			if rsp == nil {
				t.Fatalf("No response")
			}
			wg.Done()
		}()
	}

	wg.Wait()
	count := srv.CallCount("mockfunc")
	if count != 1 {
		t.Errorf("Request piggybacking failed. mockfunc should have been called 1 times, was called %d times", count)
	} else {
		t.Logf("Piggybacked %d requests", n)
	}
}
