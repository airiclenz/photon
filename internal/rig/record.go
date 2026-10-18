package rig

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

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

// quitKeyGap parts two quit keys, like any two key presses of a beat: above the 50 ms escape
// timeout common TUI input parsers use, so two presses are read as two keys.
const quitKeyGap = 100 * time.Millisecond

// takeEnv names the absolute path of the take just written, for `on_success`.
const takeEnv = "PHOTON_TAKE"

// Record records one take of loaded under the named profile ("" for the services as declared) and
// checks it, returning the check's rows. Command output (reset, on_success) and a `take: <path>`
// line once the take is written go to out.
//
// In order: the work dir is created; `reset` runs; the services start; the program launches as
// `sh -c '<launch.command>'` in a pty at the storyboard's frame, with the env of [launchEnv]; once
// it has painted, the beats run; the take ends (idle, frozen, quit keys, kill) and is written to
// the clip's take path. When every beat passed, the take is judged with the services still up and
// their ports in every `run` expect's env, the services stop, and then the profile's `on_success`
// runs. FAIL rows are rows, not an error: the caller counts them with [check.Failed].
//
// A beat that fails — or a program that exits or never paints before its first paint — still
// writes the take, then returns that error (a beat's is an *[engine.BeatError]) without checking
// the take or running `on_success`. A failure before the launch records nothing. The services are
// stopped on every path. Rows come back whenever the check ran, even when `on_success` then fails.
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
	}
	recorded, beatsErr := recording.record(ctx, slices.Concat(workVars(work), ports))
	if recorded == nil {
		return nil, stopped(beatsErr)
	}
	if err := saveTake(takePath, recorded, out); err != nil {
		return nil, stopped(errors.Join(beatsErr, err))
	}
	if beatsErr != nil {
		return nil, stopped(beatsErr)
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
// click through and the quit keys' bytes.
type recorder struct {
	board    *storyboard.Storyboard
	cfg      *config.Config
	resolver target.Resolver
	quitKeys [][]byte
}

// record launches the program with vars in its env, waits for its first paint, performs the beats
// and ends the take. It returns the take whenever the program was launched, together with the
// first thing that went wrong; a nil take means nothing was recorded.
func (r recorder) record(ctx context.Context, vars []string) (*take.Take, error) {
	environ, err := launchEnv(os.Environ(), r.cfg.Launch, vars)
	if err != nil {
		return nil, err
	}
	term, err := terminal.Start(shell, []string{shellFlag, r.cfg.Launch.Command}, terminal.Options{
		Cols: r.board.Frame.Cols, Rows: r.board.Frame.Rows, FPS: r.board.Frame.FPS,
		Env: environ, Dir: r.cfg.Launch.Dir, FromFirstPaint: true,
	})
	if err != nil {
		return nil, fmt.Errorf("launch %q: %w", r.cfg.Launch.Command, err)
	}
	beatsErr := r.performBeats(ctx, term)
	return r.endTake(ctx, term), beatsErr
}

// performBeats waits for the program's first paint, then runs the beats.
func (r recorder) performBeats(ctx context.Context, term *terminal.Terminal) error {
	timeout := time.NewTimer(firstPaintTimeout)
	defer timeout.Stop()
	select {
	case <-term.Painted():
	case <-term.Done():
		return fmt.Errorf("the program exited (status %d) before its first paint", term.ExitCode())
	case <-timeout.C:
		return fmt.Errorf("the program did not paint within %s of its launch", firstPaintTimeout)
	case <-ctx.Done():
		return ctx.Err()
	}
	return engine.New(term, r.resolver).Run(ctx, r.board.Beats)
}

// endTake ends the take after the last beat, or the failed one, and returns it. The program is
// first given up to the settle timeout to fall idle, so what it had in flight reaches the take;
// then the take is frozen, so nothing it paints on its way out is kept; then it is quit the way a
// person quits it, so it can save its state, and killed with its whole process group when it has
// not left by the kill timeout. A cancelled ctx skips the waits and goes straight to the kill.
func (r recorder) endTake(ctx context.Context, term *terminal.Terminal) *take.Take {
	end := r.cfg.End
	if ctx.Err() == nil {
		term.Settle(ctx, end.Idle, end.SettleTimeout)
	}
	term.Freeze()
	if ctx.Err() == nil {
		term.Quit(r.quitKeys, quitKeyGap, end.KillAfter)
	}
	return term.Close()
}
