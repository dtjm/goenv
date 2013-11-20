package comm

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
)

type Server struct {
	net.Listener
	routes map[string]HandlerFunc
}

type HandlerFunc func(*Request) interface{}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewServer() *Server {
	return &Server{
		routes: make(map[string]HandlerFunc),
	}
}

func (s *Server) serveConn(c net.Conn) {
	encoder := json.NewEncoder(c)
	decoder := json.NewDecoder(c)
	var req Request
	var errRsp ErrorResponse
	for {
		err := decoder.Decode(&req)
		if err != nil && err != io.EOF {
			log.Printf("%s", err)
		}
		if err != nil {
			return
		}

		rsp, handleErr := s.serveCommand(&req)
		if handleErr != nil {
			log.Printf("Handler returned error: %v", handleErr)
			errRsp.Error = handleErr.Error()
			writeErr := encoder.Encode(errRsp)
			if writeErr != nil {
				log.Printf("%s", writeErr)
				return
			}
		}
		log.Printf("rsp was: %v", rsp)

		err = encoder.Encode(rsp)
		if err != nil {
			log.Printf("%s", err)
			return
		}
	}
}

func (s *Server) ListenAndServe(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer l.Close()

	return s.Serve(l)
}
func (s *Server) Serve(l net.Listener) error {
	s.Listener = l
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}

		go func() {
			s.serveConn(conn)
			conn.Close()
		}()
	}
}

func (s *Server) Handle(cmd string, f func(*Request) interface{}) {
	if _, exists := s.routes[cmd]; exists {
		panic("Handler function already exists")
	}

	s.routes[cmd] = f
}

func (s *Server) Addr() net.Addr {
	if s.Listener != nil {
		return s.Listener.Addr()
	}
	return nil
}

func (s *Server) serveCommand(r *Request) (interface{}, error) {
	handlerFunc, exists := s.routes[r.Command]
	if !exists {
		return nil, fmt.Errorf("Invalid command")
	}

	return handlerFunc(r), nil
}
