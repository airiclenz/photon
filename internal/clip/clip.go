package clip

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/storyboard"
)

// WorkEnv is the environment variable that names the work dir when no `--work` flag does.
const WorkEnv = "PHOTON_WORK"

const (
	// cacheDir is the directory under the home dir that holds a project's default work dir.
	cacheDir = ".cache"
	// workDirSuffix follows the project's name in its default work dir's name.
	workDirSuffix = "-photon"
	// takeExtension ends the name of a clip's take file.
	takeExtension = ".take"
)

// Clip is one clip ready to record, check or render: its Storyboard and the Config of its
// project, with the work dir the rig lives in and the take path the clip's take is written to and
// read from.
//
// [Load] and [Clip.WithStoryboard] are the only ways to get one: a Clip in hand has a storyboard
// and a config that each passed validation and that agree with each other. Its work dir and take
// path resolve on demand ([Clip.Work], [Clip.Take]), so a command that needs neither, such as
// lint, never fails on them.
type Clip struct {
	Storyboard *storyboard.Storyboard
	Config     *config.Config
	// workFlag is the `--work` flag; empty when unset.
	workFlag string
}

// Load loads the storyboard at storyboardPath and the config it uses. configFlag and workFlag are
// the `--config` and `--work` flags; an empty one is unset. workFlag is only kept for
// [Clip.Work]: Load itself never resolves the work dir.
//
// The config is configFlag when set, else the nearest photon.yaml at or above the storyboard's
// directory ([config.Find]). Every problem the storyboard and the config have is reported at
// once: both errors are joined, so errors.As still reaches a *[storyboard.ValidationError] or a
// *[config.ValidationError], and errors.Is a [config.ErrNotFound]. Only when both load clean
// does Load cross-check them, reporting in a *[ValidationError] every storyboard key the config's
// launch refuses — a url launch wants frame.viewport and none of the terminal's grid keys, a
// command launch takes no viewport, device_scale or target selector — and every area the
// storyboard names but the config does not declare.
func Load(storyboardPath, configFlag, workFlag string) (*Clip, error) {
	board, boardErr := storyboard.Load(storyboardPath)
	project, configErr := loadConfig(configFlag, filepath.Dir(storyboardPath))
	if err := errors.Join(boardErr, configErr); err != nil {
		return nil, err
	}
	if problems := crossCheck(board, project); len(problems) > 0 {
		return nil, &ValidationError{Path: storyboardPath, Problems: problems}
	}
	return &Clip{Storyboard: board, Config: project, workFlag: workFlag}, nil
}

// WithStoryboard returns a clip of board with c's config and `--work` flag, cross-checked as
// [Load] does: every key the config's launch refuses and every area board names that the config
// does not declare is reported in a *[ValidationError] under board's Path. board must come from
// [storyboard.Load] or [storyboard.Parse]; c itself is left as it was.
func (c *Clip) WithStoryboard(board *storyboard.Storyboard) (*Clip, error) {
	if problems := crossCheck(board, c.Config); len(problems) > 0 {
		return nil, &ValidationError{Path: board.Path, Problems: problems}
	}
	return &Clip{Storyboard: board, Config: c.Config, workFlag: c.workFlag}, nil
}

// Work resolves the rig's work dir, always absolute: the `--work` flag, else [WorkEnv], else the
// config's `work`, else ~/.cache/<name>-photon. It fails only when the default is needed and the
// home dir cannot be found.
func (c *Clip) Work() (string, error) {
	return workDir(c.workFlag, c.Config)
}

// Take resolves the clip's take path: `<work>/<clip>.take`, with the work dir of [Clip.Work].
func (c *Clip) Take() (string, error) {
	work, err := c.Work()
	if err != nil {
		return "", err
	}
	return filepath.Join(work, c.Storyboard.Clip+takeExtension), nil
}

// loadConfig finds and loads the config a storyboard in storyboardDir uses.
func loadConfig(configFlag, storyboardDir string) (*config.Config, error) {
	file, err := config.Find(configFlag, storyboardDir)
	if err != nil {
		return nil, err
	}
	return config.Load(file)
}

// workDir resolves the rig's work dir, first set wins: the flag, [WorkEnv], the config's `work`
// (already absolute), then ~/.cache/<name>-photon. A relative flag or env value is made absolute
// against the working directory, so the take path and every command handed the work dir name
// the same place whatever directory they run in.
func workDir(workFlag string, project *config.Config) (string, error) {
	work := workFlag
	if work == "" {
		work = os.Getenv(WorkEnv)
	}
	if work == "" {
		work = project.Work
	}
	if work == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve the work dir: %w", err)
		}
		work = filepath.Join(home, cacheDir, project.Name+workDirSuffix)
	}
	absolute, err := filepath.Abs(work)
	if err != nil {
		return "", fmt.Errorf("resolve the work dir: %w", err)
	}
	return absolute, nil
}

// namedArea is one area a storyboard names, with the part of the storyboard that names it and
// the path of the field within that part.
type namedArea struct {
	owner storyboard.Owner
	field string
	area  string
}

// namedTarget is one target a storyboard names, with the part of the storyboard that names it and
// the path of the target's field within that part: empty for a camera position, whose target keys
// sit beside its own.
type namedTarget struct {
	owner  storyboard.Owner
	field  string
	target storyboard.Target
}

// keyPath is the path of the target's key, as the storyboard's own validation reports it.
func (n namedTarget) keyPath(key string) string {
	if n.field == "" {
		return key
	}
	return n.field + "." + key
}

// crossCheck returns a problem for every key the config's launch kind refuses ([launchProblems]),
// then for every area the storyboard names that the config does not declare, in storyboard order.
// An empty area and [storyboard.AreaAny] are the whole screen and need no declaration. A problem
// whose area or target a beat or a camera position names is a *[storyboard.LocatedError] filed
// under it.
func crossCheck(board *storyboard.Storyboard, project *config.Config) []error {
	problems := launchProblems(board, project)
	for _, named := range namedAreas(board) {
		if named.area == "" || named.area == storyboard.AreaAny {
			continue
		}
		if _, isDeclared := project.Areas[named.area]; !isDeclared {
			problem := fmt.Errorf(
				"%s: %q is not declared in the areas of %s", named.field, named.area, project.Path,
			)
			problems = append(problems, located(named.owner, problem))
		}
	}
	return problems
}

// launchProblems returns a problem for every storyboard key the config's launch refuses. A url
// launch films a page, so its storyboard sets frame.viewport and none of the terminal's
// frame.cols, frame.rows, frame.padding, frame.font_size and frame.line_height; a command launch
// runs a terminal, so its storyboard sets no frame.viewport, no frame.device_scale and no target
// selector.
func launchProblems(board *storyboard.Storyboard, project *config.Config) []error {
	var problems []error
	frame := board.Frame
	if project.IsBrowser() {
		if !frame.HasViewport() {
			problems = append(problems, fmt.Errorf(
				"frame.viewport: missing; the url launch of %s films a page of that size",
				project.Path))
		}
		for _, terminal := range []struct {
			name  string
			isSet bool
		}{
			{"cols", frame.Cols != 0}, {"rows", frame.Rows != 0}, {"padding", frame.Padding != 0},
			{"font_size", frame.FontSize != 0}, {"line_height", frame.LineHeight != 0},
		} {
			if terminal.isSet {
				problems = append(problems, fmt.Errorf(
					"frame.%s: a url launch takes no %s", terminal.name, terminal.name))
			}
		}
		return problems
	}
	if frame.HasViewport() {
		problems = append(problems,
			errors.New("frame.viewport: only a url launch takes a viewport"))
	}
	if frame.DeviceScale != 0 {
		problems = append(problems,
			errors.New("frame.device_scale: only a url launch takes a device_scale"))
	}
	for _, named := range namedTargets(board) {
		if named.target.Selector != "" {
			problem := fmt.Errorf("%s: only a url launch takes a selector",
				named.keyPath("selector"))
			problems = append(problems, located(named.owner, problem))
		}
	}
	return problems
}

// located files problem under owner as a *[storyboard.LocatedError], unless owner is the clip as
// a whole.
func located(owner storyboard.Owner, problem error) error {
	if owner.Kind == storyboard.OwnerClip {
		return problem
	}
	return &storyboard.LocatedError{Owner: owner, Err: problem}
}

// namedAreas lists every area field of the storyboard, set or not, under the same owners and
// field paths the storyboard's own validation reports.
func namedAreas(board *storyboard.Storyboard) []namedArea {
	var named []namedArea
	wholeClip := storyboard.Owner{Kind: storyboard.OwnerClip}
	for index, expect := range board.Expect {
		field := fmt.Sprintf("expect[%d]: area", index)
		named = append(named, namedArea{wholeClip, field, expect.Area})
	}
	for _, beat := range board.Beats {
		owner := storyboard.Owner{Kind: storyboard.OwnerBeat, Index: beat.ID}
		for index, action := range beat.Do {
			for _, target := range actionTargets(owner, index, action) {
				named = append(named, target.area())
			}
		}
		for index, expect := range beat.Expect {
			field := fmt.Sprintf("expect[%d]: area", index)
			named = append(named, namedArea{owner, field, expect.Area})
		}
	}
	for _, target := range cameraTargets(board) {
		named = append(named, target.area())
	}
	return named
}

// area is the target's area field.
func (n namedTarget) area() namedArea {
	return namedArea{n.owner, n.keyPath("area"), n.target.Area}
}

// namedTargets lists every target of the storyboard, set or not, in storyboard order: each beat's
// click and drag targets, then each camera position's.
func namedTargets(board *storyboard.Storyboard) []namedTarget {
	var named []namedTarget
	for _, beat := range board.Beats {
		owner := storyboard.Owner{Kind: storyboard.OwnerBeat, Index: beat.ID}
		for index, action := range beat.Do {
			named = append(named, actionTargets(owner, index, action)...)
		}
	}
	return append(named, cameraTargets(board)...)
}

// actionTargets lists the targets of the action at do[index]: a click's target, or a drag's from,
// each via point in turn and its to; an offset point's target is empty.
func actionTargets(owner storyboard.Owner, index int, action storyboard.Action) []namedTarget {
	if action.Click != nil {
		field := fmt.Sprintf("do[%d]: click.target", index)
		return []namedTarget{{owner, field, action.Click.Target}}
	}
	if action.Drag == nil {
		return nil
	}
	drag := *action.Drag
	field := fmt.Sprintf("do[%d]: drag.", index)
	named := make([]namedTarget, 0, len(drag.Via)+2)
	named = append(named, namedTarget{owner, field + "from", drag.From})
	for viaIndex, via := range drag.Via {
		viaField := fmt.Sprintf("%svia[%d]", field, viaIndex)
		named = append(named, namedTarget{owner, viaField, via.Target})
	}
	return append(named, namedTarget{owner, field + "to", drag.To.Target})
}

// cameraTargets lists the target of every camera position, set or not; a cell position's is
// empty.
func cameraTargets(board *storyboard.Storyboard) []namedTarget {
	named := make([]namedTarget, 0, len(board.Camera.Positions))
	for index, position := range board.Camera.Positions {
		owner := storyboard.Owner{Kind: storyboard.OwnerPosition, Index: index}
		named = append(named, namedTarget{owner, "", position.Target})
	}
	return named
}

// ValidationError lists every way a storyboard disagrees with its config.
type ValidationError struct {
	Path     string
	Problems []error
}

// Error joins the problems one per line, under the storyboard's path.
func (e *ValidationError) Error() string {
	lines := make([]string, 0, len(e.Problems)+1)
	lines = append(lines, fmt.Sprintf("%s: %d problem(s)", e.Path, len(e.Problems)))
	for _, problem := range e.Problems {
		lines = append(lines, "  "+problem.Error())
	}
	return strings.Join(lines, "\n")
}

// Unwrap exposes the individual problems to errors.Is and errors.As.
func (e *ValidationError) Unwrap() []error { return e.Problems }
