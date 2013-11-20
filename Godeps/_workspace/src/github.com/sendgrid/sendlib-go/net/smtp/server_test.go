package smtp

import (
	"bytes"
	"fmt"
	"github.com/sendgrid/sendlib-go/log"
	"io"
	"io/ioutil"
	"net"
	"net/smtp"
	"os"
	"testing"
	"testing/quick"
	"time"
)

func makeServer(t *testing.T, greeting string) (
	addr string, server *Server, client *smtp.Client, exit chan bool) {

	exit = make(chan bool)
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	addr = l.Addr().String()

	go func() {
		server = &Server{Greeting: greeting,
			ReadTimeout:     100 * time.Millisecond,
			WriteTimeout:    100 * time.Millisecond,
			ShutdownTimeout: 100 * time.Millisecond,
			Syslog:          log.NewWriterSyslog(os.Stdout),
		}
		err = server.Serve(l)
		if err != nil {
			t.Error(err)
		}
		exit <- true
	}()

	client, err = smtp.Dial(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	return
}

func TestHELO(t *testing.T) {
	greeting := "test greeting"
	_, _, client, _ := makeServer(t, greeting)

	err := client.Hello("domain.com")
	if err != nil {
		t.Fatal(err)
	}
	client.Quit()
}

func TestMailFrom(t *testing.T) {
	greeting := "test greeting"
	_, server, client, _ := makeServer(t, greeting)

	server.HandleMail = func(e *Envelope, s *Session) {
		t.Log(e)
	}

	err := client.Hello("domain.com")
	if err != nil {
		t.Fatal(err)
	}

	err = client.Mail("test@test.com")
	if err != nil {
		t.Fatal(err)
	}

	client.Quit()
}
func TestRcptTo(t *testing.T) {
	greeting := "test greeting"
	_, server, client, _ := makeServer(t, greeting)

	recipient := "test@test.com"
	server.HandleMail = func(e *Envelope, s *Session) {
		for r := range e.Recipients {
			recipient = fmt.Sprintf("<%s>", recipient)
			if r != recipient {
				t.Errorf("Incorrect recipient: %s != %s", r, recipient)
			}
		}
	}

	err := client.Hello("domain.com")
	if err != nil {
		t.Fatal(err)
	}

	err = client.Mail(recipient)
	if err != nil {
		t.Fatal(err)
	}

	err = client.Rcpt(recipient)
	if err != nil {
		t.Fatal(err)
	}
	client.Quit()
}

func TestHandler(t *testing.T) {
	addr, server, _, _ := makeServer(t, "test handler")

	from := "from@example.com"
	to := []string{"a@foo.com", "b@bar.com"}
	msg := []byte("endswith\n")

	server.HandleMail = func(e *Envelope, s *Session) {
		data, err := ioutil.ReadAll(e.Data)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(msg, data) {
			t.Fatalf("Got '%q', expected '%q'", data, msg)
		}
	}

	smtp.SendMail(addr, nil, from, to, msg)
}

func TestMultipleMails(t *testing.T) {
	_, server, client, _ := makeServer(t, "test multiple mails")

	from := "from@example.com"
	to := []string{"a@foo.com"}

	var actualData []byte
	var err error

	server.HandleMail = func(e *Envelope, s *Session) {
		actualData, err = ioutil.ReadAll(e.Data)
		if err != nil {
			t.Fatal(err)
		}
		s.WriteResponse(250, "Data received")
	}

	if err = client.Hello("multiplemails.com"); err != nil {
		t.Fatal(err)
	}

	f := func(msg []byte) bool {
		if err = client.Mail(from); err != nil {
			t.Fatal(err)
		}

		if err = client.Rcpt(to[0]); err != nil {
			t.Fatal(err)
		}

		writer, err := client.Data()
		if err != nil {
			t.Fatal(err)
		}

		// Handle weird stuff that textproto does
		if len(msg) > 0 && msg[len(msg)-1] == '\r' {
			msg[len(msg)-1] = '\n'
		}
		if len(msg) > 0 && msg[len(msg)-1] != '\n' {
			msg = append(msg, '\n')
		}

		writer.Write(msg)
		err = writer.Close()
		if err != nil {
			t.Fatalf("Error while closing writer: %s", err)
		}

		msg = bytes.Replace(msg, []byte{'\r', '\n'}, []byte{'\n'}, -1)
		ok := bytes.Equal(actualData, msg)
		if !ok {
			t.Logf("Expected '%q'", msg)
			t.Logf("got      '%q'", actualData)
		}
		return ok
	}

	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
	err = client.Quit()
	if err != nil {
		t.Fatal(err)
	}
}

func TestShutdown(t *testing.T) {
	_, server, client, exit := makeServer(t, "shutdown test")
	gotData := false
	buf := []byte{' '}
	server.HandleMail = func(e *Envelope, s *Session) {
		e.Data.Read(buf)
		server.Shutdown()
		// Sleep less than the shutdown time
		time.Sleep(server.ShutdownTimeout / 2)
		_, err := io.Copy(ioutil.Discard, e.Data)
		if err == nil {
			gotData = true
		}
	}

	client.Hello("test.local")
	client.Mail("test@test.com")
	client.Rcpt("test@test.com")
	wc, err := client.Data()
	if err != nil {
		t.Fatal(err)
	}
	_, err = wc.Write([]byte("foobar"))
	if err != nil {
		t.Fatal(err)
	}

	err = wc.Close()
	if err != io.EOF {
		t.Fatal(err)
	}

	if !gotData {
		t.Fatal("Should have gotten data before shutdown")
	}
	t.Logf("Got data")

	<-exit
}
func TestShutdownTimeout(t *testing.T) {
	_, server, client, exit := makeServer(t, "shutdown test")
	gotData := false
	buf := []byte{' '}
	server.HandleMail = func(e *Envelope, s *Session) {
		e.Data.Read(buf)
		server.Shutdown()
		// Sleep more than the shutdown time
		time.Sleep(server.ShutdownTimeout * 2)
		_, err := io.Copy(ioutil.Discard, e.Data)
		if err == nil {
			gotData = true
		}
	}

	client.Hello("test.local")
	client.Mail("test@test.com")
	client.Rcpt("test@test.com")
	wc, err := client.Data()
	if err != nil {
		t.Fatal(err)
	}
	_, err = wc.Write([]byte("foobar"))
	if err != nil {
		t.Fatal(err)
	}

	err = wc.Close()
	if err != io.EOF {
		t.Fatal(err)
	}

	if gotData {
		t.Fatal("Should not have gotten data before shutdown")
	}

	<-exit
}
