package log

import (
	"fmt"
	"io"
	"log"
	"log/syslog"
	"os"
	"regexp"
)

var defaultLog *log.Logger = log.New(os.Stderr, "", log.Llongfile)

type Syslogger interface {
	Debugf(fmt string, args ...interface{})
	Infof(fmt string, args ...interface{})
	Warnf(fmt string, args ...interface{})
	Errf(fmt string, args ...interface{})
}

type Syslog struct {
	writer *syslog.Writer
}

// An object implementing the syslog interface, but writes out to a single
// io.Writer with the severities prepended to the log message
type WriterSyslog struct {
	io.Writer
}

func NewSyslog(tag string) *Syslog {
	writer, err := syslog.New(syslog.LOG_MAIL, tag)
	if err != nil {
		defaultLog.Fatalf("Unable to create new syslog.Writer: %s", err)
	}

	return &Syslog{writer}
}

func (l *Syslog) Debugf(format string, args ...interface{}) {
	l.writer.Debug(fmt.Sprintf(format, args...))
}
func (l *Syslog) Infof(format string, args ...interface{}) {
	l.writer.Info(fmt.Sprintf(format, args...))
}
func (l *Syslog) Warnf(format string, args ...interface{}) {
	l.writer.Warning(fmt.Sprintf(format, args...))
}
func (l *Syslog) Errf(format string, args ...interface{}) {
	l.writer.Err(fmt.Sprintf(format, args...))
}

func NewWriterSyslog(w io.Writer) *WriterSyslog {
	return &WriterSyslog{w}
}

func (l *WriterSyslog) Debugf(format string, args ...interface{}) {
	l.Writer.Write([]byte(fmt.Sprintf("[dbg] "+format+"\n", args...)))
}

func (l *WriterSyslog) Infof(format string, args ...interface{}) {
	l.Writer.Write([]byte(fmt.Sprintf("[inf] "+format+"\n", args...)))
}

func (l *WriterSyslog) Warnf(format string, args ...interface{}) {
	l.Writer.Write([]byte(fmt.Sprintf("[wrn] "+format+"\n", args...)))
}

func (l *WriterSyslog) Errf(format string, args ...interface{}) {
	l.Writer.Write([]byte(fmt.Sprintf("[err] "+format+"\n", args...)))
}

// This implements the io.Writer interface by appending each []byte as an
// element in a []string
type StringWriter struct {
	Strings []string
}

func NewStringWriter() *StringWriter {
	return &StringWriter{make([]string, 0)}
}

func (w *StringWriter) Write(b []byte) (int, error) {
	w.Strings = append(w.Strings, string(b))
	return len(b), nil
}

func (w *StringWriter) Match(pattern string) bool {
	for _, s := range w.Strings {
		if re := regexp.MustCompile(pattern); re.Match([]byte(s)) {
			return true
		}
	}
	return false
}

func (w *StringWriter) Reset() {
	w.Strings = make([]string, 0)
}
