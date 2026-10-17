// Command svc is the service the service package's tests start: a small stand-in for a fake
// backend. It prints its PHOTON_* and SVC_* env, one NAME=value per line in name order, then
// optionally listens on PHOTON_PORT (answering every connection with a greeting), prints a ready
// line after a delay, or exits at once with a given status, and runs until SIGTERM, which it
// reports and exits on unless told to ignore it.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"
)

// greeting is what svc writes to every connection it accepts.
const greeting = "hello from svc\n"

// terminatedLine is what svc prints when SIGTERM stops it.
const terminatedLine = "terminated"

func main() {
	listen := flag.Bool("listen", false, "listen on 127.0.0.1:$PHOTON_PORT")
	line := flag.String("line", "", "print this line once the delay has passed")
	delay := flag.Duration("delay", 0, "wait this long before listening or printing the line")
	exitCode := flag.Int("exit", -1, "exit at once with this status")
	ignoreTerm := flag.Bool("ignore-term", false, "ignore SIGTERM")
	flag.Parse()

	printEnv()
	if *exitCode >= 0 {
		os.Exit(*exitCode)
	}

	terms := make(chan os.Signal, 1)
	if *ignoreTerm {
		signal.Ignore(syscall.SIGTERM)
	} else {
		signal.Notify(terms, syscall.SIGTERM)
	}

	time.Sleep(*delay)
	if *listen {
		if err := serve(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *line != "" {
		fmt.Println(*line)
	}

	<-terms
	fmt.Println(terminatedLine)
}

// printEnv prints every PHOTON_* and SVC_* variable, in name order.
func printEnv() {
	var lines []string
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "PHOTON_") || strings.HasPrefix(entry, "SVC_") {
			lines = append(lines, entry)
		}
	}
	slices.Sort(lines)
	for _, entry := range lines {
		fmt.Println(entry)
	}
}

// serve listens on PHOTON_PORT and greets every connection, in the background.
func serve() error {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", os.Getenv("PHOTON_PORT")))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			if _, err := connection.Write([]byte(greeting)); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
			if err := connection.Close(); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}
	}()
	return nil
}
