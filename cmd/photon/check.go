package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/airiclenz/photon/internal/check"
	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// workFlag names the flag that sets the rig's work dir.
const workFlag = "work"

// newCheckCommand judges a take against its storyboard's expects, prints one row per expect and
// exits 1 on any FAIL.
func newCheckCommand() *cobra.Command {
	var configPath, workPath string
	cmd := &cobra.Command{
		Use:   "check <storyboard> [<take>]",
		Short: "Judge a take against its storyboard's expects",
		Long: "check judges every expect of a storyboard against a take: a seen expect on the " +
			"screens\nrecorded during its beat (any screen, for a take-level expect), a run " +
			"expect by running\nits command in the work dir with PHOTON_WORK, PHOTON_TAKE and " +
			"PHOTON_BEAT set. It prints\none `beat N | PASS/FAIL | detail` row per expect and " +
			"exits 1 on any FAIL.\nThe take defaults to <work>/<clip>.take; the work dir is " +
			"--work, else $" + clip.WorkEnv + ",\nelse the config's work, else " +
			"~/.cache/<name>-photon.\nExit status: 0 when every expect passed; 1 when an expect " +
			"failed or the take could not\nbe judged; 2 for a bad command line.",
		Args: cobra.RangeArgs(1, 2),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			loaded, err := clip.Load(args[0], configPath, workPath)
			if err != nil {
				return err
			}
			rows, err := judgeClip(cmd, loaded, args[1:])
			if err != nil {
				return err
			}
			if err := check.WriteTable(cmd.OutOrStdout(), rows); err != nil {
				return err
			}
			if failed := check.Failed(rows); failed > 0 {
				return fmt.Errorf("%d expect(s) failed", failed)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&configPath, configFlag, "",
		"the photon config to use instead of the nearest photon.yaml")
	cmd.Flags().StringVar(&workPath, workFlag, "",
		"the rig's work dir, where the take is read from and run expects run")
	return cmd
}

// judgeClip loads the take — takeArgs[0] when given, else the clip's take path — and judges it.
func judgeClip(cmd *cobra.Command, loaded *clip.Clip, takeArgs []string) ([]check.Row, error) {
	work, err := loaded.Work()
	if err != nil {
		return nil, err
	}
	var takePath string
	if len(takeArgs) > 0 {
		takePath = takeArgs[0]
	} else if takePath, err = loaded.Take(); err != nil {
		return nil, err
	}
	recorded, err := take.Load(takePath)
	if err != nil {
		return nil, err
	}
	resolver, err := target.New(loaded.Config.Areas)
	if err != nil {
		return nil, err
	}
	return check.Judge(
		cmd.Context(), loaded.Storyboard, recorded, resolver, work, takePath, check.Options{})
}
