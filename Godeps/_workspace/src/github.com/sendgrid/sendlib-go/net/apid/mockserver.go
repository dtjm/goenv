package apid

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
)

type mockFunc func() (interface{}, *Error)

type MockServer struct {
	FunctionMeta  map[string]*functionMetadata
	MockFunctions map[string]mockFunc
	CallHistory   []mockCall
	URL           string
	*http.ServeMux
	*httptest.Server
}

type mockCall struct {
	Function string
	Args     url.Values
}

func NewMockServer() *MockServer {

	mux := http.NewServeMux()
	testServer := httptest.NewServer(mux)

	s := &MockServer{
		FunctionMeta:  make(map[string]*functionMetadata),
		MockFunctions: make(map[string]mockFunc),
		CallHistory:   make([]mockCall, 0),
		Server:        testServer,
		ServeMux:      mux,
		URL:           testServer.URL[7:],
	}

	mux.HandleFunc("/api/functions.json", func(rw http.ResponseWriter, r *http.Request) {
		log.Printf("functions.json: %v", s.FunctionMeta)
		rw.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(rw)
		wrapper := make(map[string]map[string]*functionMetadata)
		wrapper["functions"] = s.FunctionMeta
		enc.Encode(wrapper)

		b, _ := json.Marshal(wrapper)
		log.Printf("%s", b)
	})
	return s
}

func (s *MockServer) CallCount(function string) int {
	n := 0
	for _, call := range s.CallHistory {
		if call.Function == function {
			n++
		}
	}

	return n
}

func (s *MockServer) NextCall() mockCall {
	call := s.CallHistory[0]
	s.CallHistory = s.CallHistory[1:]
	return call
}

func (s *MockServer) MockFunction(function string, cacheTTL int, f mockFunc) {
	s.MockFunctions[function] = f

	s.FunctionMeta[function] = &functionMetadata{
		Function:  function,
		ResultKey: "result",
		Path:      "/mock/" + function,
		Cachable:  cacheTTL,
		Params:    make(map[string]string),
	}
	log.Printf("MockFunction: %v", s.FunctionMeta)

	s.ServeMux.HandleFunc("/mock/"+function, func(rw http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		s.CallHistory = append(s.CallHistory, mockCall{function, r.Form})
		rw.Header().Set("Content-Type", "application/json")

		v, apidErr := f()

		encoder := json.NewEncoder(rw)
		if apidErr != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			encoder.Encode(apidErr)
			return
		}

		wrapper := make(map[string]interface{})
		wrapper["result"] = v
		encoder.Encode(wrapper)
	})

	log.Printf("%v", s)
}
