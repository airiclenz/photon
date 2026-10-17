package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/airiclenz/photon"
)

// newVersionCommand builds `photon version`, which prints photon's version and nothing else.
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print photon's version",
		Long:  "Print photon's version, such as 1.0.0, read from the VERSION file it was built from.",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, _ []string) error {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), photon.Version()); err != nil {
				return fmt.Errorf("print the version: %w", err)
			}
			return nil
		}),
	}
}
