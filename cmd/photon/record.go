package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/airiclenz/photon/internal/check"
	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/rig"
)

// profileFlag names the flag that picks the profile a take's services run under.
const profileFlag = "profile"

// newRecordCommand records one take of a clip, checks it, prints the check table and exits with
// the take's status, so a retake loop is `until photon record <storyboard>; do :; done`.
func newRecordCommand() *cobra.Command {
	var configPath, workPath, profile string
	cmd := &cobra.Command{
		Use:   "record <storyboard>",
		Short: "Record one take of a clip and check it",
		Long: "record runs the config's reset, starts the take's services (under --profile when given),\n" +
			"launches the program in a pty at the storyboard's frame, performs every beat, ends the\n" +
			"take and writes it to <work>/<clip>.take. When every beat passed, the take is checked\n" +
			"with the services still up, the check table is printed, the services stop and the\n" +
			"profile's on_success runs. A take whose beats failed is still written, but not checked.\n" +
			"The commands' output and the take's path go to stderr; the check table to stdout.\n" +
			"The work dir is --work, else $" + clip.WorkEnv + ", else the config's work, else\n" +
			"~/.cache/<name>-photon. Recording needs a unix pty: it refuses on Windows.\n" +
			"Exit status: 0 when every beat and expect passed; 1 when a beat failed, an expect\n" +
			"failed or the take could not be recorded; 2 for a bad command line.",
		Args: cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			if err := refuseWithoutPTY(); err != nil {
				return err
			}
			loaded, err := clip.Load(args[0], configPath, workPath)
			if err != nil {
				return err
			}
			rows, err := rig.Record(cmd.Context(), loaded, profile, cmd.ErrOrStderr())
			return reportTake(cmd, rows, err)
		}),
	}
	cmd.Flags().StringVar(&configPath, configFlag, "", "the photon config to use instead of the nearest photon.yaml")
	cmd.Flags().StringVar(&workPath, workFlag, "", "the rig's work dir, where the take is written and run expects run")
	cmd.Flags().StringVar(&profile, profileFlag, "", "the profile whose services and on_success the take runs with")
	return cmd
}

// reportTake prints the check table when the take was checked and returns the take's failure:
// recordErr, joined with a count of the expects that failed.
func reportTake(cmd *cobra.Command, rows []check.Row, recordErr error) error {
	if rows == nil {
		return recordErr
	}
	if err := check.WriteTable(cmd.OutOrStdout(), rows); err != nil {
		return errors.Join(recordErr, err)
	}
	if failed := check.Failed(rows); failed > 0 {
		return errors.Join(fmt.Errorf("%d expect(s) failed", failed), recordErr)
	}
	return recordErr
}
