package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/airiclenz/photon/internal/config"
)

const (
	// shell and shellFlag run a service's command line, like every command photon runs.
	shell     = "sh"
	shellFlag = "-c"
	// workEnv carries the work dir to every service; it is the variable clip reads the work dir
	// from, so a service and a later command agree on it.
	workEnv = "PHOTON_WORK"
	// portEnv carries a service's own port to it.
	portEnv = "PHOTON_PORT"
	// logExtension ends the name of a service's log file in the work dir.
	logExtension = ".log"
	// stopTimeout is how long a service's group is given to exit after SIGTERM before it is killed.
	stopTimeout = 5 * time.Second
	// stopPollInterval is how long Stop pauses between two checks that a group has exited.
	stopPollInterval = 10 * time.Millisecond
	// waitDelay bounds how long reaping a service waits for its output to close once it has
	// exited, when a child it forked still holds the pipe.
	waitDelay = time.Second
	// freePortHost is where a free port for a service is asked from the kernel.
	freePortHost = "127.0.0.1:0"
	// maxPortAttempts bounds the tries for a free port that no other service has fixed.
	maxPortAttempts = 10
)

// ErrUnknownProfile is what [Start] returns, wrapped with the declared profiles, for a profile
// the config does not declare.
var ErrUnknownProfile = errors.New("unknown profile")

// ErrNotReady is what [Start] returns, wrapped with the service's name and log path, for a
// service that exited or ran out of time before it was ready.
var ErrNotReady = errors.New("not ready")

// Set is the running services of one take, in the order they started. A nil Set has no services.
type Set struct {
	services []*running
	env      []string
}

// running is one started service: its process and the group it leads, its log, and its exit
// once reaped.
type running struct {
	name    string
	log     string
	cmd     *exec.Cmd
	group   *group
	exited  chan struct{}
	waitErr error
}

// planned is one service to start: its name and what it runs under the chosen profile.
type planned struct {
	name    string
	service config.Service
}

// Start starts every service cfg declares, in name order, under the named profile: an empty
// profile runs each service as declared; a declared one swaps the command and adds the env its
// overrides give. Each runs as `sh -c <command>` in cfg.Dir, in its own process group, with the
// operator's env plus its expanded config env, and with PHOTON_WORK set to work, PHOTON_PORT to
// its own port and the PHOTON_PORT_<NAME> of every service started before it; its output, both
// streams, goes to `<work>/<name>.log`. A port of 0 is a free one the kernel picks.
//
// Start returns once every service is ready, or the first error: an [ErrUnknownProfile] listing
// the declared profiles, an [ErrNotReady] naming the service and its log path, or a failure to
// start one. On an error the services already started are stopped before it returns, and the Set
// is nil. ctx bounds only the start; the services run until [Set.Stop].
func Start(ctx context.Context, cfg *config.Config, profile, work string) (*Set, error) {
	services, err := plan(cfg, profile)
	if err != nil {
		return nil, err
	}

	taken := fixedPorts(cfg.Services)
	set := &Set{}
	for _, service := range services {
		if err := set.start(ctx, cfg.Dir, work, service, taken); err != nil {
			return nil, errors.Join(err, set.Stop())
		}
	}
	return set, nil
}

// Env lists every started service's port as PHOTON_PORT_<NAME>=<port>, in start order, for the
// launch and the commands that follow it. A nil Set lists none.
func (s *Set) Env() []string {
	if s == nil {
		return nil
	}
	return slices.Clone(s.env)
}

// Stop stops every service, the last started first: SIGTERM to its process group, then a kill
// once the group has not exited within the stop timeout. It reports each service it could not
// kill; a service's exit status, signalled or its own, is not an error. Stop on a nil Set does
// nothing, so a caller may defer it whether or not Start succeeded.
func (s *Set) Stop() error {
	if s == nil {
		return nil
	}
	var errs []error
	for _, service := range slices.Backward(s.services) {
		if err := service.stop(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// plan returns the services to start, in name order, with the profile's overrides applied.
func plan(cfg *config.Config, profile string) ([]planned, error) {
	var overrides map[string]config.ServiceOverride
	if profile != "" {
		chosen, isDeclared := cfg.Profiles[profile]
		if !isDeclared {
			return nil, fmt.Errorf("%w %q; declared: %s",
				ErrUnknownProfile, profile, declared(cfg.Profiles))
		}
		overrides = chosen.Services
	}

	services := make([]planned, 0, len(cfg.Services))
	for _, name := range slices.Sorted(maps.Keys(cfg.Services)) {
		service := cfg.Services[name]
		override := overrides[name]
		service.Command = cmp.Or(override.Command, service.Command)
		if len(override.Env) > 0 {
			env := maps.Clone(service.Env)
			if env == nil {
				env = make(map[string]string, len(override.Env))
			}
			maps.Copy(env, override.Env)
			service.Env = env
		}
		services = append(services, planned{name: name, service: service})
	}
	return services, nil
}

// declared lists the profile names, in name order, or says there are none.
func declared(profiles map[string]config.Profile) string {
	if len(profiles) == 0 {
		return "none"
	}
	return strings.Join(slices.Sorted(maps.Keys(profiles)), ", ")
}

// fixedPorts returns the set of ports the config fixes, which a picked port must not take.
func fixedPorts(services map[string]config.Service) map[int]bool {
	taken := make(map[int]bool, len(services))
	for _, service := range services {
		if service.Port != 0 {
			taken[service.Port] = true
		}
	}
	return taken
}

// start starts one service, waits until it is ready, and adds it to the set; a service started
// but not ready is still added, so that Stop stops it.
func (s *Set) start(ctx context.Context, dir, work string, next planned, taken map[int]bool) error {
	port, err := portFor(next.service.Port, taken)
	if err != nil {
		return fmt.Errorf("pick a port for service %s: %w", next.name, err)
	}
	vars := append(slices.Clone(s.env), workEnv+"="+work, portEnv+"="+strconv.Itoa(port))
	env, err := ExpandEnv(next.service.Env, vars)
	if err != nil {
		return fmt.Errorf("service %s: %w", next.name, err)
	}
	environ := slices.Concat(os.Environ(), env, vars)

	service, ready, err := launch(dir, work, next, port, environ)
	if err != nil {
		return err
	}
	s.services = append(s.services, service)
	timeout := cmp.Or(next.service.Timeout, config.DefaultServiceTimeout)
	if err := awaitReady(ctx, service, ready, timeout); err != nil {
		return err
	}
	s.env = append(s.env, config.PortEnvName(next.name)+"="+strconv.Itoa(port))
	return nil
}

// portFor returns the service's fixed port, or for 0 a free one the kernel picks that no service
// has fixed, and marks it taken.
func portFor(fixed int, taken map[int]bool) (int, error) {
	if fixed != 0 {
		return fixed, nil
	}
	for range maxPortAttempts {
		port, err := freePort()
		if err != nil {
			return 0, err
		}
		if !taken[port] {
			taken[port] = true
			return port, nil
		}
	}
	return 0, fmt.Errorf("every free port the kernel offered in %d tries is a fixed one",
		maxPortAttempts)
}

// freePort asks the kernel for a free loopback port and releases it for the service to take.
func freePort() (int, error) {
	listener, err := net.Listen("tcp", freePortHost)
	if err != nil {
		return 0, fmt.Errorf("ask for a free port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, fmt.Errorf("release the free port %d: %w", port, err)
	}
	return port, nil
}

// launch starts the service's process with its output in its log, and reaps it in the
// background. It returns the readiness check to wait on.
func launch(
	dir, work string, next planned, port int, environ []string,
) (*running, readiness, error) {
	var ready readiness
	var pattern *regexp.Regexp
	if next.service.Ready.Line != "" {
		compiled, err := regexp.Compile(next.service.Ready.Line)
		if err != nil {
			return nil, ready, fmt.Errorf("service %s: ready.line: %w", next.name, err)
		}
		pattern = compiled
	} else {
		ready = tcpReadiness(port)
	}

	logPath := filepath.Join(work, next.name+logExtension)
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, ready, fmt.Errorf("create service %s's log: %w", next.name, err)
	}

	cmd := exec.Command(shell, shellFlag, next.service.Command)
	cmd.Dir = dir
	cmd.Env = environ
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if pattern != nil {
		watcher := newLineWatcher(logFile, pattern)
		cmd.Stdout, cmd.Stderr = watcher, watcher
		ready.matched = watcher.matched
	}
	cmd.WaitDelay = waitDelay
	prepareGroup(cmd)
	if err := cmd.Start(); err != nil {
		startErr := fmt.Errorf("start service %s: %w", next.name, err)
		return nil, ready, errors.Join(startErr, logFile.Close())
	}
	group, err := joinGroup(cmd.Process)
	if err != nil {
		return nil, ready, errors.Join(fmt.Errorf("start service %s: %w", next.name, err),
			cmd.Process.Kill(), cmd.Wait(), logFile.Close())
	}

	service := &running{
		name: next.name, log: logPath, cmd: cmd, group: group, exited: make(chan struct{}),
	}
	go service.reap(logFile)
	return service, ready, nil
}

// reap waits for the service to exit, closes its log and marks it exited.
func (r *running) reap(logFile *os.File) {
	waitErr := r.cmd.Wait()
	if err := logFile.Close(); err != nil {
		waitErr = errors.Join(waitErr, fmt.Errorf("close the log: %w", err))
	}
	r.waitErr = waitErr
	close(r.exited)
}

// stop asks the service's group to exit and kills it once it has not within stopTimeout, then
// releases the group.
func (r *running) stop() error {
	if err := r.group.terminate(); err == nil && r.awaitGone(stopTimeout) {
		return r.releaseGroup()
	}
	if err := r.group.kill(); err != nil {
		return errors.Join(fmt.Errorf("kill service %s: %w", r.name, err), r.releaseGroup())
	}
	<-r.exited
	return r.releaseGroup()
}

// releaseGroup frees what the service's group holds.
func (r *running) releaseGroup() error {
	if err := r.group.release(); err != nil {
		return fmt.Errorf("release service %s's group: %w", r.name, err)
	}
	return nil
}

// awaitGone reports whether the service has exited, and every process of its group with it,
// within timeout.
func (r *running) awaitGone(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if isClosed(r.exited) && !r.group.alive() {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(stopPollInterval)
	}
}
