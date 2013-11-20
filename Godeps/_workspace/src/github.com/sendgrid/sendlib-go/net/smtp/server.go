package smtp

import (
	"bufio"
	"fmt"
	"github.com/sendgrid/sendlib-go/log"
	"io"
	"net"
	"net/textproto"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SessionState string

const (
	NewConnection    SessionState = "NEW_CONNECTION"
	GreetingSent     SessionState = "GREETING_SENT"
	ReadyForMail     SessionState = "READY_FOR_MAIL"
	MailFromReceived SessionState = "MAIL_FROM_RECEIVED"
	RcptToReceived   SessionState = "RCPT_TO_RECEIVED"
	DataReceiving    SessionState = "DATA_RECEIVING"
)

type ServerState string
type EmailAddress string

const (
	Listening    ServerState = "LISTENING"
	ShuttingDown ServerState = "SHUTTING_DOWN"
)

type Envelope struct {
	MailFrom EmailAddress

	// Recipient addresses will be provided one by one.
	Recipients chan EmailAddress

	// The reader which can be used to stream the data from the client.
	Data io.ReadCloser

	pipeWriter io.WriteCloser
}

type Session struct {
	id uint64
	Envelope
	State SessionState
	net.Conn
	*Server

	// A reader which wraps the net.Conn. Note that this is buffered
	*textproto.Reader

	// The Domain reported by the client when saying HELO
	HeloDomain string
}

func NewSession(conn net.Conn, server *Server) *Session {
	return &Session{
		id:     server.Stats.Connections,
		State:  NewConnection,
		Conn:   conn,
		Reader: textproto.NewReader(bufio.NewReader(conn)),
		Server: server}
}

func (s *Session) WriteResponse(code int, reason string) error {
	s.Conn.SetWriteDeadline(time.Now().Add(s.Server.WriteTimeout))
	_, err := fmt.Fprintf(s.Conn, "%d %s\r\n", code, reason)
	return err
}

func (s *Session) handleNewConnection() {
	s.Conn.SetWriteDeadline(
		time.Now().Add(s.Server.WriteTimeout))
	s.WriteResponse(220, s.Server.Greeting)
	s.Server.Syslog.Debugf("Wrote greeting: %s", s.Server.Greeting)
	s.State = GreetingSent
}

func (s *Session) expectHelo() (ok bool) {
	if s.Server.State == ShuttingDown {
		s.Server.Syslog.Infof("Server shutdown detected in expectHelo")
		return false
	}

	s.Server.Syslog.Debugf("Reading HELO")
	cmd, arg, err := s.nextClientCommand()

	if err != nil {
		return s.handleReadError(err)
	}

	switch cmd {
	case "HELO", "EHLO":
		if err = ValidateDomain(arg); err != nil {
			s.WriteResponse(501, "Syntax error in parameters or arguments: "+err.Error())
			return true
		}

		s.State = ReadyForMail
		s.HeloDomain = arg
		s.WriteResponse(250, "OK")
		return true
	case "QUIT":
		s.WriteResponse(250, "OK")
		return false
	default:
		s.WriteResponse(503, "Where are your manners?")
		return true
	}
}

func (s *Session) expectMailFrom() bool {
	if s.Server.State == ShuttingDown {
		s.Server.Syslog.Infof(
			"Server shutdown detected in expectMailFrom")
		return false
	}

	s.Envelope.Recipients = make(chan EmailAddress)
	s.Envelope.Data, s.Envelope.pipeWriter = io.Pipe()

	cmd, arg, err := s.nextClientCommand()

	if err != nil {
		return s.handleReadError(err)
	}

	switch cmd {

	case "QUIT":
		s.WriteResponse(221,
			fmt.Sprintf("%s Service closing transmission channel", s.Domain))
		return false

	case "RSET":
		return true

	case "MAIL FROM":
		if err = ValidateEmail(arg); err != nil {
			s.WriteResponse(501, "Syntax error in parameters or arguments")
			return true
		}
		s.Envelope.MailFrom = EmailAddress(arg)
		s.WriteResponse(250, "OK")
		s.State = MailFromReceived
		if s.Server.HandleMail != nil {
			go s.Server.HandleMail(&s.Envelope, s)
		}
		return true
	default:
		s.WriteResponse(500, "Expected MAIL FROM")
		return true
	}

	panic("Failed to hit default case")
}

func (s *Session) expectRcptTo() bool {
	if s.Server.State == ShuttingDown {
		s.Server.Syslog.Infof(
			"Server shutdown detected in expectMailFrom")
		return false
	}

	s.Server.Syslog.Debugf("MAIL FROM received...")
	cmd, arg, err := s.nextClientCommand()

	if err != nil {
		return s.handleReadError(err)
	}

	switch cmd {
	case "RCPT TO":
		if err = ValidateEmail(arg); err != nil {
			s.WriteResponse(501, "Syntax error in parameters or arguments")
			return true
		}

		// Send recipient to channel if someone is receiving
		select {
		case s.Envelope.Recipients <- EmailAddress(arg):
			s.Server.Syslog.Debugf("Sending %s to chan", arg)
		default:
			s.Server.Syslog.Debugf("Discarding recipient")
			s.WriteResponse(250, "OK")
		}
		s.State = RcptToReceived
		return true

	case "RSET":
		s.WriteResponse(250, "OK")
		s.State = ReadyForMail
		return true

	default:
		s.WriteResponse(500, "Expected RCPT TO")
		return true
	}
}

func (s *Session) handleReadError(err error) bool {
	if err == nil {
		panic("Only pass non-nil errors here, please")
	}
	if err == io.EOF {
		s.Server.Syslog.Debugf("Client disconnected addr=%s", s.Conn.RemoteAddr().String())
		return false
	}
	if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
		s.Server.Syslog.Debugf("Connection timed out: state=%s addr=%s helo=%s",
			s.State, s.Conn.RemoteAddr().String(), s.HeloDomain)
		return false
	}
	s.Server.Syslog.Debugf("Error: '%s' state=%s addr=%s helo=%s", err,
		s.State, s.Conn.RemoteAddr().String(), s.HeloDomain)
	return false

}

func (s *Session) expectData() (ok bool) {
	if s.Server.State == ShuttingDown {
		s.Server.Syslog.Infof(
			"Server shutdown detected in expectData")
		return false
	}

	cmd, arg, err := s.nextClientCommand()

	if err != nil {
		return s.handleReadError(err)
	}

	switch cmd {
	case "RCPT TO":
		if arg == "" {
			s.WriteResponse(501, "Syntax error in argument")
			return true
		}

		// Try to send the recipient through the channel, or just drop it if nobody is listening
		select {
		case s.Envelope.Recipients <- EmailAddress(arg):
			s.Server.Syslog.Debugf("Sending additional %s to chan", arg)
		default:
			s.Server.Syslog.Debugf("Discarding recipient")
		}
		s.WriteResponse(250, "OK")
		return true

	case "DATA":
		close(s.Envelope.Recipients)
		s.WriteResponse(354, "Start mail input; end with <CRLF>.<CRLF>")
		s.State = DataReceiving
		return true

	default:
		s.WriteResponse(503, "Bad sequence of commands. Try RCPT TO or DATA.")
		return true
	}
}

var dataBufSize int = 4 * 1024

// Return from this function to close the session (and close the client connection)
func (s *Session) serve() {
	// Make a buffer we can reuse for reading data off the wire
	dataBuf := make([]byte, dataBufSize)
	for {
		s.Server.Syslog.Debugf("Transitioned state: %s", s.State)

		switch s.State {
		case NewConnection:
			s.handleNewConnection()
		case GreetingSent:
			if ok := s.expectHelo(); !ok {
				return
			}
		case ReadyForMail:
			if ok := s.expectMailFrom(); !ok {
				return
			}
		case MailFromReceived:
			if ok := s.expectRcptTo(); !ok {
				return
			}
		case RcptToReceived:
			if ok := s.expectData(); !ok {
				return
			}
		case DataReceiving:
			dotReader := s.Reader.DotReader()

			totalBytesRead := 0
			for {
				s.Conn.SetReadDeadline(time.Now().Add(s.Server.ReadTimeout))
				n, err := dotReader.Read(dataBuf)
				totalBytesRead += n
				if err != nil && err != io.EOF {
					s.Server.Syslog.Debugf("Error while reading DATA: %s\n", err)
					return
				}

				s.Server.Syslog.Debugf("Read bytes from client: %q", dataBuf[:n])

				// Reached the end of data since we didn't fill the buffer
				s.Envelope.pipeWriter.Write(dataBuf[:n])
				if n == 0 || n < dataBufSize || err == io.EOF {
					break
				}
			}
			s.Server.Syslog.Debugf("Read %d bytes in DATA", totalBytesRead)
			s.Envelope.pipeWriter.Close()
			s.State = ReadyForMail
		default:
			panic("Session entered unknown state")
		}
	}
}

var clientCommandRegexp *regexp.Regexp = regexp.MustCompile("(?i)(helo|ehlo|mail from|data|rcpt to|rset|quit|vrfy]):?\\s*(.{0,255})?")

func (s *Session) nextClientCommand() (cmd, arg string, err error) {
	s.Conn.SetReadDeadline(time.Now().Add(s.Server.ReadTimeout))
	line, err := s.Reader.ReadLine()
	s.Server.Syslog.Debugf("line from client: %s", line)

	if err != nil {
		return
	}

	matches := clientCommandRegexp.FindStringSubmatch(line)
	s.Server.Syslog.Debugf("parsed line: %#v", matches)
	switch len(matches) {
	case 0, 1:
	case 2:
		cmd = matches[1]
	case 3:
		cmd = matches[1]
		arg = matches[2]
	}
	cmd = strings.ToUpper(strings.Trim(cmd, " "))
	return
}

type Server struct {
	Domain          string
	Greeting        string
	ReadTimeout     time.Duration // maximum duration before timing out read of the request
	WriteTimeout    time.Duration // maximum duration before timing out write of the response
	ShutdownTimeout time.Duration
	MaxDataBytes    int // maximum size of the data segment of the message
	State           ServerState
	Syslog          log.Syslogger
	HandleMail      func(*Envelope, *Session)
	Stats           struct {
		Connections       uint64 `json:"connections"`
		MessagesCompleted uint64 `json:"messages_completed"`
	}
	listeners []net.Listener
	wg        sync.WaitGroup
	shutdown  chan bool
}

var maxTempDelay time.Duration = 1 * time.Second

func (s *Server) Serve(l net.Listener) error {
	if s.listeners == nil {
		s.listeners = make([]net.Listener, 0, 1)
	}

	s.listeners = append(s.listeners, l)

	if s.shutdown == nil {
		s.shutdown = make(chan bool)
	}

	var tempDelay time.Duration // how long to sleep on accept failure
	s.Syslog.Infof("Starting to serve on %s", l.Addr().String())
	if s.Greeting == "" {
		s.Greeting = fmt.Sprintf("%s Service ready", s.Domain)
	}

	connTable := make(map[net.Conn]struct{})

	errChan := make(chan error)
	connChan := make(chan net.Conn)

LOOP:
	for {
		go func() {
			if conn, e := l.Accept(); e != nil {
				errChan <- e
			} else {
				connChan <- conn
			}
		}()

		select {
		case e := <-errChan:
			if e != nil {
				if ne, ok := e.(net.Error); ok && ne.Temporary() {
					if tempDelay == 0 {
						tempDelay = 5 * time.Millisecond
					} else {
						tempDelay *= 2
					}
					if tempDelay > maxTempDelay {
						tempDelay = maxTempDelay
					}
					s.Syslog.Infof("smtp: Accept error: %v retrying in %v", e, tempDelay)
					time.Sleep(tempDelay)
					continue
				}
				s.Syslog.Debugf("State: %s", s.State)
				return e
			}

		case conn := <-connChan:
			s.wg.Add(1)

			connTable[conn] = struct{}{}
			atomic.AddUint64(&s.Stats.Connections, 1)

			tempDelay = 0

			session := NewSession(conn, s)
			go func() {
				session.serve()
				s.Syslog.Debugf("Session closing")
				conn.Close()
				s.wg.Done()
				delete(connTable, conn)
			}()

		case <-s.shutdown:
			s.Syslog.Infof("Server received shutdown signal")
			break LOOP
		}

	}

	wgUnblocked := make(chan bool)
	go func() {
		s.wg.Wait()
		wgUnblocked <- true
	}()

	select {
	case <-wgUnblocked:
		s.Syslog.Infof("All sessions completed")
	case <-time.After(s.ShutdownTimeout):
		s.Syslog.Infof("Timed out waiting for sessions to complete")
		for conn, _ := range connTable {
			s.Syslog.Infof("Forcing connection closed: %s", conn.RemoteAddr().String())
			conn.Close()
		}
	}
	return nil
}

// Start a graceful shutdown
func (s *Server) Shutdown() {
	if s.State == ShuttingDown {
		s.Syslog.Infof("Received redundant shutdown request")
		return
	}

	close(s.shutdown)

	for _, l := range s.listeners {
		err := l.Close()
		if err != nil {
			s.Syslog.Errf("Error closing listener: %s", err)
		}
	}

	s.State = ShuttingDown
}

var DomainRegexp *regexp.Regexp = regexp.MustCompile("[a-zA-Z\\.]+")

func ValidateDomain(domain string) error {
	if len(domain) > 255 {
		return fmt.Errorf("Domain '%s...' longer than 255 chars", domain[:255])
	}

	if !DomainRegexp.Match([]byte(domain)) {
		return fmt.Errorf("Domain '%s' does not look like a domain name", domain)
	}

	return nil
}

var EmailRegexp *regexp.Regexp = regexp.MustCompile("\\<?(.+)@([^>]+)\\>?")

func ValidateEmail(email string) error {
	if !EmailRegexp.Match([]byte(email)) {
		return fmt.Errorf("Doesn't look like an email address")
	}

	return nil
}

func (a EmailAddress) Domain() string {
	matches := EmailRegexp.FindStringSubmatch(string(a))
	fmt.Printf("Matches: %+v\n", matches)
	return matches[2]
}
