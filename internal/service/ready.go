package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"time"
)

const (
	// readyPollInterval is how long the wait for readiness pauses between two TCP dials.
	readyPollInterval = 20 * time.Millisecond
	// dialTimeout bounds one TCP dial of the readiness check.
	dialTimeout = 200 * time.Millisecond
	// readyHost is the host a TCP readiness check dials: the loopback, by name, so a service
	// listening on either IPv4 or IPv6 counts.
	readyHost = "localhost"
	// maxPendingLine is the longest partial output line kept while watching for the ready line;
	// a longer one cannot be a readiness line and is dropped.
	maxPendingLine = 64 << 10
)

// lineWatcher passes a service's output through to its log and closes matched the first time a
// whole line matches pattern. exec writes to it from one goroutine at a time, since a service's
// stdout and stderr are the same watcher.
type lineWatcher struct {
	out       io.Writer
	pattern   *regexp.Regexp
	pending   []byte
	matched   chan struct{}
	isMatched bool
}

// newLineWatcher returns a watcher writing to out and matching lines against pattern.
func newLineWatcher(out io.Writer, pattern *regexp.Regexp) *lineWatcher {
	return &lineWatcher{out: out, pattern: pattern, matched: make(chan struct{})}
}

// Write writes p to the log and, until a line has matched, scans it for whole lines.
func (w *lineWatcher) Write(p []byte) (int, error) {
	written, err := w.out.Write(p)
	if !w.isMatched {
		w.scan(p[:written])
	}
	return written, err
}

// scan adds p to the partial line held and checks every line p completes; a trailing \r is not
// part of a line.
func (w *lineWatcher) scan(p []byte) {
	w.pending = append(w.pending, p...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			break
		}
		line := bytes.TrimSuffix(w.pending[:end], []byte("\r"))
		if w.pattern.Match(line) {
			w.isMatched = true
			w.pending = nil
			close(w.matched)
			return
		}
		w.pending = w.pending[end+1:]
	}
	if len(w.pending) > maxPendingLine {
		w.pending = nil
	}
}

// readiness is how a started service tells it is up: matched closes when its ready line was seen,
// and address, when set, is the port that accepts a connection once it is up. Exactly one is set.
type readiness struct {
	matched <-chan struct{}
	address string
}

// tcpReadiness is the readiness of a service whose port accepts a connection once it is up.
func tcpReadiness(port int) readiness {
	return readiness{address: net.JoinHostPort(readyHost, strconv.Itoa(port))}
}

// accepts reports whether the readiness check's address takes a connection now; a check by line
// never does.
func (r readiness) accepts() bool {
	if r.address == "" {
		return false
	}
	connection, err := net.DialTimeout("tcp", r.address, dialTimeout)
	if err != nil {
		return false
	}
	connection.Close() //nolint:errcheck // the probe reached the service, all readiness asks
	return true
}

// awaitReady waits until service is ready, its process exits, timeout passes or ctx is done,
// whichever comes first; only the first is a nil error. A service that is not ready is named
// with its log path in the error.
func awaitReady(ctx context.Context, service *running, ready readiness, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	poll := time.NewTicker(readyPollInterval)
	defer poll.Stop()

	for {
		if ready.accepts() {
			return nil
		}
		select {
		case <-ready.matched:
			return nil
		case <-service.exited:
			if isClosed(ready.matched) || ready.accepts() {
				return nil
			}
			return fmt.Errorf("%w: service %s exited before it was ready (%v); its log is %s",
				ErrNotReady, service.name, service.waitErr, service.log)
		case <-deadline.C:
			return fmt.Errorf("%w: service %s within %s; its log is %s",
				ErrNotReady, service.name, timeout, service.log)
		case <-ctx.Done():
			return fmt.Errorf("wait for service %s: %w; its log is %s", service.name, ctx.Err(), service.log)
		case <-poll.C:
		}
	}
}

// isClosed reports whether the channel is closed; a nil channel never is.
func isClosed(channel <-chan struct{}) bool {
	select {
	case <-channel:
		return true
	default:
		return false
	}
}
