package parseapi

import (
	"fmt"
	sglog "github.com/sendgrid/sendlib-go/log"
	"github.com/sendgrid/sendlib-go/net/apid"
	"github.com/sendgrid/sendlib-go/net/smtp"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

var hostname string

type Server struct {
	ApidClient        *apid.Client
	Syslog            sglog.Syslogger
	smtpServer        *smtp.Server
	postQueue         chan *postJob
	WorkerConcurrency uint
	Version           string
}

type postJob struct {
	MessageID string
	*apid.ParseHostSettings
	Attempts uint
}

func (s *Server) HandleMail(e *smtp.Envelope, session *smtp.Session) {
	recipients := make(map[smtp.EmailAddress]*apid.ParseHostSettings)
	for rcpt := range e.Recipients {
		rcptDomain := rcpt.Domain()

		s.Syslog.Debugf("Received recipient domain: %s", rcptDomain)

		parseSettings, err := s.ApidClient.GetParseHostSettings(rcptDomain)

		if err != nil || parseSettings.UserID == 0 {
			session.WriteResponse(550, "Mailbox unavailable")
			session.Close()
			return
		}

		session.WriteResponse(250, "Recipient ok")

		s.Syslog.Debugf("Parse settings for recipient %s: %+v", rcpt, parseSettings)
		recipients[rcpt] = parseSettings
	}

	// Create a message ID
	messageID := fmt.Sprintf("%s.%s.%s",
		hostname,
		strconv.FormatInt(time.Now().UnixNano(), 36),
		strconv.FormatInt(rand.Int63n(2<<61), 36))

	// Copy data to disk
	filepath := "/var/spool/parsed/incoming/" + messageID
	f, err := os.Create(filepath)
	if err != nil {
		session.WriteResponse(451, "Requested action aborted: local error in processing")
		s.Syslog.Errf("%s", err)
		session.Close()
		return
	}

	n, err := io.Copy(f, e.Data)
	if err != nil {
		session.WriteResponse(451, "Requested action aborted: local error in processing")
		s.Syslog.Errf("%s, deleting %s", err, messageID)
		session.Close()
		f.Close()
		os.Remove(filepath)
		return
	}

	err = f.Sync()
	if err != nil {
		s.Syslog.Errf("Error syncing file: %s", err)
	}

	err = f.Close()
	if err != nil {
		s.Syslog.Errf("Error closing file: %s", err)
	}

	err = session.WriteResponse(250, fmt.Sprintf("Queued message %s", messageID))

	if err != nil {
		s.Syslog.Infof("Error writing response to client, removing message: %s %s", messageID, err)
		os.Remove(filepath)
		return
	}

	s.Syslog.Infof("Queued message %s (%d bytes)", messageID, n)

	// Put message on the post queue
	go func() {
		for _, parseSettings := range recipients {
			s.postQueue <- &postJob{
				MessageID:         messageID,
				ParseHostSettings: parseSettings,
			}
		}
	}()
}
func (s *Server) startPostWorkers() {
	for i := uint(0); i < s.WorkerConcurrency; i++ {
		go func() {
			for job := range s.postQueue {
				data, err := ioutil.ReadFile("/var/spool/parsed/incoming/" + job.MessageID)
				if err != nil {
					s.Syslog.Errf("Error reading message, requeueing: '%s' %+v", err, job)
					s.QueueJob(job)
					continue
				}
				form := url.Values{}
				form.Set("email", string(data))
				rsp, err := http.PostForm(job.ParseHostSettings.URL, form)

				if err != nil {
					s.Syslog.Errf("Error posting, requeueing: '%s' job: %+v", err, job)
					s.QueueJob(job)
					continue
				}

				if rsp.StatusCode >= 500 {
					s.QueueJob(job)
					continue
				}

				if rsp.StatusCode >= 400 {
					s.Syslog.Infof("Endpoint returned '%s', dropping job: %+v",
						rsp.Status, *job)
					continue
				}

				if rsp.StatusCode >= 200 {
					s.Syslog.Infof("Post success '%s': job: %+v", rsp.Status, *job)
					continue
				}
			}
		}()
	}
}
func (s *Server) QueueJob(job *postJob) {
	job.Attempts++
	if job.Attempts > 10 {
		s.Syslog.Warnf("Job exceeded max retries, dropping: %+v", *job)
		return
	}
	delay := time.Duration(math.Exp(float64(job.Attempts))) * time.Second
	s.Syslog.Infof("Scheduling job to be retried in %0.2f seconds: %+v", delay.Seconds(), job)

	time.AfterFunc(delay, func() {
		s.Syslog.Infof("Putting job back on the queue: %+v", job)
		s.postQueue <- job
	})
	return
}

func (s *Server) ListenAndServe(smtpAddr string, httpAddr string) error {
	if s.postQueue == nil {
		s.postQueue = make(chan *postJob, 1000)
	}

	s.smtpServer = &smtp.Server{
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		Greeting:     fmt.Sprintf("Parse API %s", s.Version),
		Syslog:       s.Syslog,
		HandleMail:   s.HandleMail,
	}

	smtpListen, err := net.Listen("tcp", smtpAddr)
	if err != nil {
		return err
	}

	exitChan := make(chan error)

	go func() {
		s.Syslog.Infof("Listening on public smtp://%s", smtpAddr)
		err := s.smtpServer.Serve(smtpListen)
		if err != nil {
			s.Syslog.Errf("Failed to start SMTP service: %s", err)
		}
		exitChan <- err
	}()

	go func() {
		s.Syslog.Infof("Listening on management http://%s", httpAddr)
		err := http.ListenAndServe(httpAddr, nil)
		if err != nil {
			s.Syslog.Errf("Failed to start management service: %s", err)
		}
		exitChan <- err
	}()

	go s.startPostWorkers()

	return <-exitChan
}

// Perform a graceful shutdown, with a forceful termination after `timeout` duration
func (s *Server) Shutdown(timeout time.Duration) {
	s.smtpServer.Shutdown(timeout)
}

func init() {
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		panic(err)
	}
}
