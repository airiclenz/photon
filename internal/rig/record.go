package rig

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/airiclenz/photon/internal/browser"
	"github.com/airiclenz/photon/internal/check"
	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/engine"
	"github.com/airiclenz/photon/internal/keys"
	"github.com/airiclenz/photon/internal/service"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
	"github.com/airiclenz/photon/internal/terminal"
)

// firstPaintTimeout bounds how long a take waits for the program's first paint after its launch.
const firstPaintTimeout = 30 * time.Second

// takeEnv names the absolute path of the take just written, for `on_success`.
const takeEnv = "PHOTON_TAKE"

// urlReference is a `${NAME}` reference in a launch url; group 1 is the variable name.
var urlReference = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Record records one take of loaded under the named profile ("" for the services as declared) and
// checks it, returning the check's rows. Command output (reset, on_success) and a `take: <path>`
// line once the take is written go to out.
//
// In order: the work dir is created; `reset` runs; the services start; the program launches as
// `sh -c '<launch.command>'` in a pty at the storyboard's frame, with the env of [launchEnv] — or,
// on a url launch, headless Chromium opens `launch.url`, its `${NAME}` references expanded from
// that env, at the frame's viewport, device scale and fps, recording the boxes of every selector
// the clip names; once it has painted, the beats run; the take ends (idle, frozen, then quit keys
// and kill for a program, a close for the browser) and is written to the clip's take path. When
// every beat passed, the take is judged with the services still up and their ports in every `run`
// expect's env, the services stop, and then the profile's `on_success` runs. FAIL rows are rows,
// not an error: the caller counts them with [check.Failed].
//
// A beat that fails — or a program that exits or never paints before its first paint, or a
// browser that fails to record or close — still writes the take, then returns that error (a
// beat's is an *[engine.BeatError]) without checking the take or running `on_success`. A failure
// before the launch records nothing. The services are stopped on every path. Rows come back
// whenever the check ran, even when `on_success` then fails.
func Record(
	ctx context.Context, loaded *clip.Clip, profile string, out io.Writer,
) ([]check.Row, error) {
	cfg := loaded.Config
	onSuccess, err := onSuccessOf(cfg, profile)
	if err != nil {
		return nil, err
	}
	quitKeys, err := keySequences(cfg.End.Keys)
	if err != nil {
		return nil, err
	}
	resolver, err := target.New(cfg.Areas)
	if err != nil {
		return nil, err
	}
	work, err := prepareWork(loaded)
	if err != nil {
		return nil, err
	}
	takePath, err := loaded.Take()
	if err != nil {
		return nil, err
	}
	if err := runCommand(ctx, "reset", cfg.Reset, cfg.Dir, workVars(work), out); err != nil {
		return nil, err
	}

	services, err := service.Start(ctx, cfg, profile, work)
	if err != nil {
		return nil, err
	}
	stopped := func(err error) error { return errors.Join(err, services.Stop()) }
	ports := services.Env()
	recording := recorder{
		board: loaded.Storyboard, cfg: cfg, resolver: resolver, quitKeys: quitKeys,
		selectors: loaded.Selectors(),
	}
	recorded, takeErr := recording.record(ctx, slices.Concat(workVars(work), ports))
	if recorded == nil {
		return nil, stopped(takeErr)
	}
	if err := saveTake(takePath, recorded, out); err != nil {
		return nil, stopped(errors.Join(takeErr, err))
	}
	if takeErr != nil {
		return nil, stopped(takeErr)
	}

	rows, err := check.Judge(
		ctx, loaded.Storyboard, recorded, resolver, work, takePath, check.Options{Env: ports})
	if err := stopped(err); err != nil {
		return nil, err
	}
	vars := append(workVars(work), takeEnv+"="+takePath)
	err = runCommand(ctx, "on_success", onSuccess, cfg.Dir, slices.Concat(vars, ports), out)
	if err != nil {
		return rows, err
	}
	return rows, nil
}

// onSuccessOf is the named profile's `on_success`, "" when it has none or no profile is named.
// A profile the config does not declare is a [service.ErrUnknownProfile], reported before reset
// runs.
func onSuccessOf(cfg *config.Config, profile string) (string, error) {
	if profile == "" {
		return "", nil
	}
	chosen, isDeclared := cfg.Profiles[profile]
	if !isDeclared {
		declared := slices.Sorted(maps.Keys(cfg.Profiles))
		return "", fmt.Errorf("%w %q; declared: %s",
			service.ErrUnknownProfile, profile, strings.Join(declared, ", "))
	}
	return chosen.OnSuccess, nil
}

// keySequences turns the end-of-take key names into the bytes each sends.
func keySequences(names []string) ([][]byte, error) {
	sequences := make([][]byte, 0, len(names))
	for _, name := range names {
		sequence, isKnown := keys.Lookup(name)
		if !isKnown {
			return nil, fmt.Errorf("end.keys: unknown key %q", name)
		}
		sequences = append(sequences, sequence)
	}
	return sequences, nil
}

// saveTake writes recorded to takePath and names it on out.
func saveTake(takePath string, recorded *take.Take, out io.Writer) error {
	if err := take.Save(takePath, recorded); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "take: %s\n", takePath); err != nil {
		return fmt.Errorf("report the take: %w", err)
	}
	return nil
}

// recorder is what recording one take needs: the storyboard, the config, the resolver the beats
// click through, the quit keys' bytes and, for a url launch, the selectors whose boxes each
// snapshot records.
type recorder struct {
	board     *storyboard.Storyboard
	cfg       *config.Config
	resolver  target.Resolver
	quitKeys  [][]byte
	selectors []string
}

// record launches the program or opens the page with vars in its env, waits for its first paint,
// performs the beats and ends the take. It returns the take whenever the launch succeeded,
// together with the first thing that went wrong — a failed beat, then a failure ending the take;
// a nil take means nothing was recorded.
func (r recorder) record(ctx context.Context, vars []string) (*take.Take, error) {
	environ, err := launchEnv(os.Environ(), r.cfg.Launch, vars)
	if err != nil {
		return nil, err
	}
	launched, err := r.launch(ctx, environ)
	if err != nil {
		return nil, err
	}
	beatsErr := r.performBeats(ctx, launched)
	recorded, endErr := r.endTake(ctx, launched)
	return recorded, errors.Join(beatsErr, endErr)
}

// launch starts the take's capture with environ as its env: headless Chromium at the launch's url
// on a url launch, else the launch's command in a pty.
func (r recorder) launch(ctx context.Context, environ []string) (capture, error) {
	if r.cfg.IsBrowser() {
		return r.openPage(ctx, environ)
	}
	return r.startProgram(environ)
}

// startProgram runs the launch's command as `sh -c '<command>'` in a pty of the frame's cols and
// rows, in the launch dir with environ as its env.
func (r recorder) startProgram(environ []string) (capture, error) {
	term, err := terminal.Start(shell, []string{shellFlag, r.cfg.Launch.Command}, terminal.Options{
		Cols: r.board.Frame.Cols, Rows: r.board.Frame.Rows, FPS: r.board.Frame.FPS,
		Env: environ, Dir: r.cfg.Launch.Dir, FromFirstPaint: true,
	})
	if err != nil {
		return nil, fmt.Errorf("launch %q: %w", r.cfg.Launch.Command, err)
	}
	return terminalCapture{
		Terminal:  term,
		quitKeys:  r.quitKeys,
		killAfter: r.cfg.End.KillAfter,
	}, nil
}

// openPage starts headless Chromium — `launch.browser` when set — at the launch's url, its
// `${NAME}` references expanded from environ, with the frame's viewport, device scale and fps and
// environ added to the browser's env, recording the boxes of r's selectors. The browser lives as
// long as ctx.
func (r recorder) openPage(ctx context.Context, environ []string) (capture, error) {
	address, err := launchURL(r.cfg.Launch.URL, environ)
	if err != nil {
		return nil, err
	}
	frame := r.board.Frame
	page, err := browser.Start(ctx, browser.Options{
		URL:         address,
		ExecPath:    r.cfg.Launch.Browser,
		Width:       frame.Viewport.Width,
		Height:      frame.Viewport.Height,
		DeviceScale: frame.Scaling(),
		FPS:         frame.FPS,
		Selectors:   r.selectors,
		Env:         environ,
	})
	if err != nil {
		return nil, fmt.Errorf("launch.url: %w", err)
	}
	return browserCapture{Browser: page}, nil
}

// launchURL is raw with every `${NAME}` reference replaced by that variable's value in environ, a
// list of environ entries in which a later entry wins. A reference to a variable environ does not
// hold is an error naming it, so a typo or a service's missing port never reaches the browser as
// an empty string. Nothing else is expanded, and the values go in as they are.
func launchURL(raw string, environ []string) (string, error) {
	values := make(map[string]string, len(environ))
	for _, entry := range environ {
		name, value, _ := strings.Cut(entry, "=")
		values[name] = value
	}
	var missing string
	expanded := urlReference.ReplaceAllStringFunc(raw, func(reference string) string {
		name := urlReference.FindStringSubmatch(reference)[1]
		value, isSet := values[name]
		if !isSet && missing == "" {
			missing = name
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("launch.url: ${%s} is not set here", missing)
	}
	return expanded, nil
}

// performBeats waits for the capture's first paint, then runs the beats.
func (r recorder) performBeats(ctx context.Context, launched capture) error {
	timeout := time.NewTimer(firstPaintTimeout)
	defer timeout.Stop()
	select {
	case <-launched.Painted():
	case <-launched.Done():
		return fmt.Errorf("%w before its first paint", launched.exitError())
	case <-timeout.C:
		return fmt.Errorf("the program did not paint within %s of its launch", firstPaintTimeout)
	case <-ctx.Done():
		return ctx.Err()
	}
	return engine.New(launched, r.resolver).Run(ctx, r.board.Beats)
}

// endTake ends the take after the last beat, or the failed one, and returns it with any error the
// capture met finishing it. The program is first given up to the settle timeout to fall idle, so
// what it had in flight reaches the take; then the take is frozen, so nothing it paints on its
// way out is kept; then the capture is finished: a program in a pty is quit the way a person
// quits it, so it can save its state, and killed with its whole process group when it has not
// left by the kill timeout; a browser is closed. A cancelled ctx skips the waits and goes
// straight to the kill.
func (r recorder) endTake(ctx context.Context, launched capture) (*take.Take, error) {
	end := r.cfg.End
	if ctx.Err() == nil {
		launched.Settle(ctx, end.Idle, end.SettleTimeout)
	}
	launched.Freeze()
	return launched.finish(ctx)
}
