package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/airiclenz/photon"
)

// newVersionCommand builds `photon version`, which prints photon's build string and nothing else.
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print photon's version and the commit it was built from",
		Long: "Print photon's build string, such as \"photon 1.0.0 (a1b2c3d)\": the version " +
			"read from the\nVERSION file it was built from, the short commit and \", dirty\" " +
			"when the tree had\nuncommitted changes. A binary built without VCS info prints " +
			"\"photon 1.0.0\".",
		Args: cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, _ []string) error {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), photon.Build()); err != nil {
				return fmt.Errorf("print the version: %w", err)
			}
			return nil
		}),
	}
}
