package tune

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/airiclenz/photon/internal/clip"
)

// Options are a tuning run's settings: Take is the take to tune against, the clip's own take
// when empty, Port the port to listen on, any free one when 0, and Build the build string the
// page shows, the one `photon version` prints.
type Options struct {
	Take  string
	Port  int
	Build string
}

// listenHost is the one address the server listens on: the page is for this machine only.
const listenHost = "127.0.0.1"

// urlFormat is the line Run prints once it listens, naming the page's address.
const urlFormat = "photon tune: http://%s/\n"

// errorLogPrefix starts each line the HTTP server itself logs to stderr.
const errorLogPrefix = "photon tune: "

// The server's timeouts. A frame can wait on a first rasterization, so writes get the longest.
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = time.Minute
	idleTimeout       = 2 * time.Minute
	// shutdownTimeout bounds how long requests in flight may finish once ctx ends.
	shutdownTimeout = 5 * time.Second
)

// Run tunes loaded against its take: it starts a [Session] that reports options.Build, listens
// on 127.0.0.1 at options.Port only, prints `photon tune: http://127.0.0.1:<port>/` to stdout,
// and serves the page and its API (see [newHandler]) until ctx ends, then shuts the server down
// and returns nil. The HTTP server's own errors go to stderr. It fails when the session cannot
// start, when the port cannot be listened on, and when serving stops on its own.
func Run(
	ctx context.Context,
	loaded *clip.Clip,
	options Options,
	stdout, stderr io.Writer,
) error {
	session, err := NewSession(loaded, options.Take)
	if err != nil {
		return err
	}
	session.photonBuild = options.Build
	address := net.JoinHostPort(listenHost, strconv.Itoa(options.Port))
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", address, err)
	}
	return serve(ctx, listener, session, stdout, stderr)
}

// serve serves session on listener until ctx ends, after printing the page's address.
func serve(
	ctx context.Context,
	listener net.Listener,
	session *Session,
	stdout, stderr io.Writer,
) error {
	addr := listener.Addr().String()
	httpServer := &http.Server{
		Handler:           newHandler(session, addr),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          log.New(stderr, errorLogPrefix, 0),
	}
	if _, err := fmt.Fprintf(stdout, urlFormat, addr); err != nil {
		return errors.Join(fmt.Errorf("print the address: %w", err), listener.Close())
	}

	served := make(chan error, 1)
	go func() { served <- httpServer.Serve(listener) }()
	select {
	case err := <-served:
		return fmt.Errorf("serve on %s: %w", addr, err)
	case <-ctx.Done():
	}

	stopping, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(stopping); err != nil {
		return fmt.Errorf("shut the server down: %w", err)
	}
	return nil
}
