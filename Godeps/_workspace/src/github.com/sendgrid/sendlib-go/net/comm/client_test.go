package comm

import (
	"log"
	"sync"
	"testing"
	"time"
)

type Foo struct {
	Foo string
	Bar int
}

var wg sync.WaitGroup

func TestNoArgs(t *testing.T) {
	s := NewServer()
	myFoo := Foo{"Hello", 123456}
	s.Handle("test", func(r *Request) interface{} {
		t.Logf("In HandleFunc: %v", r)
		return myFoo
	})

	go func() {
		err := s.ListenAndServe(":")
		if err != nil {
			t.Error(err.Error())
		}
	}()

	time.Sleep(100 * time.Millisecond)
	c, err := Dial(s.Addr().String())
	if err != nil {
		log.Fatal(err)
	}

	req := &Request{Command: "test"}
	foo := Foo{}
	rsp, err := c.Do(req, &foo)
	if err != nil {
		log.Fatalf(err.Error())
	}
	log.Printf("%v %v", rsp, foo)
}
