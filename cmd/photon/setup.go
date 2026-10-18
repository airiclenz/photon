package main

import (
	"github.com/spf13/cobra"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/rig"
)

// newSetupCommand runs the config's one-off `setup`, which builds the rig a take records in.
func newSetupCommand() *cobra.Command {
	var configPath, workPath string
	cmd := &cobra.Command{
		Use:   "setup <storyboard>",
		Short: "Run the config's setup once, to build the rig",
		Long: "setup creates the work dir and runs the config's setup command once, by sh -c in " +
			"the\nconfig's dir with PHOTON_WORK set; its output goes to stdout. A config with no " +
			"setup\ncreates the work dir and runs nothing. The work dir is resolved as for " +
			"record.\nsetup refuses on Windows, where nothing can be recorded.\nExit status: 0 " +
			"when setup succeeded; 1 when it failed; 2 for a bad command line.",
		Args: cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			if err := refuseWithoutPTY(); err != nil {
				return err
			}
			loaded, err := clip.Load(args[0], configPath, workPath)
			if err != nil {
				return err
			}
			return rig.Setup(cmd.Context(), loaded, cmd.OutOrStdout())
		}),
	}
	cmd.Flags().StringVar(&configPath, configFlag, "",
		"the photon config to use instead of the nearest photon.yaml")
	cmd.Flags().StringVar(&workPath, workFlag, "", "the rig's work dir, created before setup runs")
	return cmd
}
