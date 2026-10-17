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
// and a config that each passed validation and that agree with each other. Its work dir and take path resolve on demand
// ([Clip.Work], [Clip.Take]), so a command that needs neither, such as lint, never fails on them.
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
// does Load cross-check them, reporting every area the storyboard names but the config does not
// declare in a *[ValidationError].
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
// [Load] does: every area board names that the config does not declare is reported in a
// *[ValidationError] under board's Path. board must come from [storyboard.Load] or
// [storyboard.Parse]; c itself is left as it was.
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

// namedArea is one area a storyboard names, with the path of the field that names it.
type namedArea struct {
	field string
	area  string
}

// crossCheck returns a problem for every area the storyboard names that the config does not
// declare, in storyboard order. An empty area and [storyboard.AreaAny] are the whole screen and
// need no declaration.
func crossCheck(board *storyboard.Storyboard, project *config.Config) []error {
	var problems []error
	for _, named := range namedAreas(board) {
		if named.area == "" || named.area == storyboard.AreaAny {
			continue
		}
		if _, isDeclared := project.Areas[named.area]; !isDeclared {
			problems = append(problems, fmt.Errorf(
				"%s: %q is not declared in the areas of %s", named.field, named.area, project.Path,
			))
		}
	}
	return problems
}

// namedAreas lists every area field of the storyboard, set or not, under the same field paths
// the storyboard's own validation reports.
func namedAreas(board *storyboard.Storyboard) []namedArea {
	var named []namedArea
	for index, expect := range board.Expect {
		named = append(named, namedArea{fmt.Sprintf("expect[%d]: area", index), expect.Area})
	}
	for _, beat := range board.Beats {
		prefix := fmt.Sprintf("beat %d: ", beat.ID)
		if beat.Zoom != nil {
			named = append(named, namedArea{prefix + "zoom.target.area", beat.Zoom.Target.Area})
		}
		for index, action := range beat.Do {
			if action.Click != nil {
				field := fmt.Sprintf("%sdo[%d]: click.target.area", prefix, index)
				named = append(named, namedArea{field, action.Click.Target.Area})
			}
			if action.Drag != nil {
				field := fmt.Sprintf("%sdo[%d]: drag.", prefix, index)
				named = append(named,
					namedArea{field + "from.area", action.Drag.From.Area},
					namedArea{field + "to.area", action.Drag.To.Area},
				)
			}
		}
		for index, expect := range beat.Expect {
			field := fmt.Sprintf("%sexpect[%d]: area", prefix, index)
			named = append(named, namedArea{field, expect.Area})
		}
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
