package main

import (
	"github.com/sendgrid/go-parseapid/parseapi"
	"github.com/sendgrid/sendlib-go/cache"
	sglog "github.com/sendgrid/sendlib-go/log"
	"github.com/sendgrid/sendlib-go/net/apid"
	"log"
	"math/rand"
	"os"
	"time"
)

const (
	VERSION string = "0.0.1"
)

var hostname string

func main() {
	rand.Seed(time.Now().UnixNano())
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		log.Fatal(err)
	}

	// Create logger
	syslogger := sglog.NewWriterSyslog(os.Stderr)

	// Create byteCache for apid
	byteCache := cache.NewMemoryCache(2<<20, 2<<24)

	// Create apid client
	apidClient, err := apid.NewClient("127.0.0.1:8082", byteCache, syslogger)
	if err != nil {
		log.Fatal(err)
	}

	// Create server
	server := parseapi.Server{
		ApidClient:        apidClient,
		Syslog:            syslogger,
		WorkerConcurrency: 2,
		Version:           VERSION,
	}

	err = server.ListenAndServe(":25", ":6970")
	if err != nil {
		log.Fatal(err)
	}
}
