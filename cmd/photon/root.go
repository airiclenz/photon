package main

import (
	"github.com/spf13/cobra"

	"github.com/airiclenz/photon"
)

// programName is the command's name: its usage line and the prefix of every error it prints.
const programName = "photon"

// versionTemplate is what --version prints: the bare version, the same line `photon version` prints.
const versionTemplate = "{{.Version}}\n"

// newRootCommand builds the command tree. Errors are silenced here and printed by execute; usage
// still prints for a bad command line, because that is the one failure the usage text answers.
func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   programName,
		Short: "Record, check and render a terminal app's demo clip from its storyboard",
		Long: "photon records a terminal app's demo clip from a storyboard, checks the take, and\n" +
			"renders the shipped GIF. A project plugs in through its photon config and storyboards.\n" +
			"Every command finds the config by --config, else the nearest photon.yaml at or above\n" +
			"the storyboard. The retake loop is `until photon record <storyboard>; do :; done`.\n" +
			"Exit status: 0 on success; 1 when a run started and failed; 2 for a bad command line.",
		Version:       photon.Version(),
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}
	cmd.SetVersionTemplate(versionTemplate)
	cmd.AddCommand(newLintCommand())
	cmd.AddCommand(newSetupCommand())
	cmd.AddCommand(newRecordCommand())
	cmd.AddCommand(newCheckCommand())
	cmd.AddCommand(newRenderCommand())
	cmd.AddCommand(newTuneCommand())
	cmd.AddCommand(newVersionCommand())
	return cmd
}
