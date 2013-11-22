package main

import (
	"fmt"
	flag "github.com/ogier/pflag"
	"github.com/sendgrid/go-parseapid/parseapi"
	"github.com/sendgrid/sendlib-go/cache"
	"github.com/sendgrid/sendlib-go/config"
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

var configFile *string = flag.String("config", "/etc/sendgrid/parseapid.conf", "Config file")

func main() {
	flag.Parse()

	// Create logger
	syslogger := sglog.NewWriterSyslog(os.Stdout, os.Stderr,
		fmt.Sprintf("parseapid[%d] ", os.Getpid()), log.LstdFlags|log.Lmicroseconds)

	cfg, err := config.NewFromFile(*configFile)
	if err != nil {
		syslogger.Errf("Unable to load config file '%s': %s", *configFile, err)
		os.Exit(1)
	}

	// Seed the RNG for Message-ID generation
	rand.Seed(time.Now().UnixNano())

	// Create byteCache for apid
	byteCache := cache.NewMemoryCache(2 << 20)

	apidAddr := fmt.Sprintf("%s:%d",
		cfg.GetString("parseapid.APID_SERVER", "127.0.0.1"),
		cfg.GetInt("parseapid.APID_PORT", 8082))

	// Create apid client
	apidClient, err := apid.NewClient(apidAddr, byteCache, syslogger)
	if err != nil {
		log.Fatal(err)
	}

	// Create server
	server := parseapi.Server{
		ApidClient:        apidClient,
		Syslog:            syslogger,
		Version:           VERSION,
		WorkerConcurrency: 2,
	}

	smtpAddr := fmt.Sprintf("%s:%d",
		cfg.GetString("parseapid.SMTP_INTERFACE", "127.0.0.1"),
		cfg.GetInt("parseapid.SMTP_PORT", 25))

	httpAddr := fmt.Sprintf("%s:%d",
		cfg.GetString("parseapid.MANAGEMENT_INTERFACE", "127.0.0.1"),
		cfg.GetInt("parseapid.MANAGEMENT_PORT", 6970))

	exit := make(chan int)
	go func() {
		err = server.ListenAndServe(smtpAddr, httpAddr)
		if err != nil {
			syslogger.Errf("Error starting parseapid server: %s", err)
			exit <- 1
		}
	}()

	// Listen for SIGTERM and initiate graceful shutdown if we get one
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, syscall.SIGTERM)
	go func() {
		<-sigChan
		syslogger.Infof("Received SIGTERM")
		server.Shutdown(10 * time.Second)
		exit <- 0
	}()

	code := <-exit
	os.Exit(code)
}
