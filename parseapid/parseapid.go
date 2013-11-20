package main

import (
	"github.com/sendgrid/go-parseapid/parseapi"
	"github.com/sendgrid/sendlib-go/cache"
	sglog "github.com/sendgrid/sendlib-go/log"
	"github.com/sendgrid/sendlib-go/net/apid"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
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

	pid := os.Getpid()
	syslogger.Infof("Starting with PID %d", pid)

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

	exit := make(chan int)
	sigChan := make(chan os.Signal, 1)
	go func() {
		err = server.ListenAndServe(":25", ":6970")
		if err != nil {
			syslogger.Errf("Error starting parseapid server: %s", err)
			exit <- 1
		}
	}()

	signal.Notify(sigChan, syscall.SIGTERM)

	go func() {
		for sig := range sigChan {
			switch sig {
			case syscall.SIGTERM:
				server.Shutdown(10 * time.Second)
				exit <- 0
			}
		}
	}()

	code := <-exit
	os.Exit(code)
}
