package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// portEnv names the variable holding the port the fixture listens on.
const portEnv = "PHOTON_PORT"

// listenHost is the loopback address the fixture listens on.
const listenHost = "127.0.0.1"

// readyLine is what the fixture prints on stdout once it listens.
const readyLine = "ready"

// pagePath is the one path the fixture serves its page at.
const pagePath = "/"

// readHeaderTimeout bounds how long a client may take to send its request headers.
const readHeaderTimeout = 5 * time.Second

// errNoPort reports that portEnv is unset or empty.
var errNoPort = errors.New(portEnv + " is not set")

// page is the fixture page. Typing in the input shows "you typed: <text>" on the status line; a
// click on a nav item shows "selected: <item>"; a click on the Run button shows "working" beside
// a spinning spinner for pendingMillis, then removes the spinner and shows "clicked: Run", so the
// page goes still again. A second click while the work is pending restarts it.
const page = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>fixture web</title>
<style>
body { margin: 16px; font-family: monospace; font-size: 16px; background: #fff; color: #000; }
nav a { margin-right: 16px; color: #00c; }
p { margin: 12px 0; }
#spinner {
  display: inline-block; width: 12px; height: 12px; margin-left: 8px;
  border: 2px solid #888; border-top-color: transparent; border-radius: 50%;
  animation: spin 0.6s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }
</style>
</head>
<body>
<h1>fixture web</h1>
<nav>
<a href="#alpha" data-item="alpha">alpha</a>
<a href="#beta" data-item="beta">beta</a>
<a href="#gamma" data-item="gamma">gamma</a>
</nav>
<p><input id="input" type="text" autofocus> <button id="run" type="button">Run</button></p>
<p id="status">ready</p>
<script>
const pendingMillis = 1000;
const status = document.getElementById("status");
const input = document.getElementById("input");
const run = document.getElementById("run");
let pending = null;

input.addEventListener("input", () => { status.textContent = "you typed: " + input.value; });

for (const link of document.querySelectorAll("nav a")) {
  link.addEventListener("click", (event) => {
    event.preventDefault();
    status.textContent = "selected: " + link.dataset.item;
  });
}

run.addEventListener("click", () => {
  clearTimeout(pending);
  if (!document.getElementById("spinner")) {
    const spinner = document.createElement("span");
    spinner.id = "spinner";
    run.after(spinner);
  }
  status.textContent = "working";
  pending = setTimeout(() => {
    document.getElementById("spinner").remove();
    status.textContent = "clicked: " + run.textContent;
    pending = null;
  }, pendingMillis);
});
</script>
</body>
</html>
`

func main() {
	if err := serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// serve listens on listenHost:$PHOTON_PORT, prints readyLine and serves the page until the
// process is stopped.
func serve() error {
	port := os.Getenv(portEnv)
	if port == "" {
		return errNoPort
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(listenHost, port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	fmt.Println(readyLine)

	server := &http.Server{
		Handler:           http.HandlerFunc(servePage),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	if err := server.Serve(listener); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// servePage answers a request for pagePath with the page and any other path with 404.
func servePage(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != pagePath {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := writer.Write([]byte(page)); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
