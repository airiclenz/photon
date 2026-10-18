package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/render"
)

// configFlag names the flag that points a command at a photon config instead of the nearest one.
const configFlag = "config"

// newLintCommand checks one storyboard and the config it uses, and reports every problem both
// files have, or that the clip is ok. Once both load clean it lays the captions out with the
// clip's real fonts, so a caption box larger than the frame is reported here, not first by render.
func newLintCommand() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "lint <storyboard>",
		Short: "Check a storyboard and its photon config without recording",
		Long: "lint loads a storyboard and the photon config it uses (--config, else the nearest\n" +
			"photon.yaml at or above the storyboard), and prints every problem the two files have,\n" +
			"or ok with the clip's name, beat count and config path. Once both are clean it lays the\n" +
			"captions out with the config's fonts and reports a caption box larger than the frame.\n" +
			"It records nothing.\n" +
			"Exit status: 0 when both files are clean; 1 when either has a problem; 2 for a bad\n" +
			"command line.",
		Args: cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			loaded, err := clip.Load(args[0], configPath, "")
			if err != nil {
				return err
			}
			if err := render.CheckCaptions(loaded); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: ok — clip %s, %d beat(s), config %s\n",
				args[0], loaded.Storyboard.Clip, len(loaded.Storyboard.Beats), loaded.Config.Path)
			return err
		}),
	}
	cmd.Flags().StringVar(&configPath, configFlag, "", "the photon config to use instead of the nearest photon.yaml")
	return cmd
}
