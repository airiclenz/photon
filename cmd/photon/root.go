package main

import "github.com/spf13/cobra"

// programName is the command's name: its usage line and the prefix of every error it prints.
const programName = "photon"

// newRootCommand builds the command tree. Errors are silenced here and printed by execute; usage
// still prints for a bad command line, because that is the one failure the usage text answers.
func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   programName,
		Short: "Record, check and render a terminal app's demo clip from its storyboard",
		Long: "photon records a terminal app's demo clip from a storyboard, checks the take, and\n" +
			"renders the shipped GIF. A project plugs in through its photon config and storyboards.",
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}
	cmd.AddCommand(newLintCommand())
	cmd.AddCommand(newSetupCommand())
	cmd.AddCommand(newRecordCommand())
	cmd.AddCommand(newCheckCommand())
	cmd.AddCommand(newRenderCommand())
	return cmd
}
