package comm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

var l *log.Logger = log.New(os.Stderr, "[sendlib/command] ", log.Lshortfile)

// A Command Server client (aka Jenkins-RPC)
type Client struct {
	Timeout time.Duration
	net.Conn
	BufReader *bufio.Reader
	BufWriter *bufio.Writer
	*json.Encoder
}

type Args map[string]interface{}

type Request struct {
	Command string `json:"cmd"`
	Args
}

func (r *Request) MarshalJSON() ([]byte, error) {
	if r.Args == nil {
		return []byte(fmt.Sprintf(`{"cmd":"%s"}`, r.Command)), nil
	}
	args, err := json.Marshal(r.Args)
	if err != nil {
		log.Printf("Error in MarshalJSON: %s", err)
		return nil, err
	}
	log.Printf("Args: %v %T", args, args)
	data := []byte(fmt.Sprintf(`{"command":"%s",%s`, r.Command, args))
	log.Printf("Made JSON: %s", data)
	return data, nil
}

type Response json.RawMessage

func Dial(addr string) (c *Client, err error) {
	c = new(Client)
	c.Conn, err = net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	c.BufReader = bufio.NewReader(c.Conn)
	c.Encoder = json.NewEncoder(c.Conn)

	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) SetTimeout(t time.Duration) {
	c.Timeout = t
}

// Performs a request. If out is assigned, decodes the JSON response into the
// given variable.
func (c *Client) Do(req *Request, out interface{}) (*Response, error) {
	err := c.Encoder.Encode(req)
	if err != nil {
		log.Printf("Error in Do: %s", err)
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	// Read the line that comes back
	line, err := c.BufReader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	if out != nil {
		err = json.Unmarshal(line, out)
		if err != nil {
			return nil, err
		}
	}

	return (*Response)(&line), nil
}
