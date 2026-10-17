package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	os.Exit(execute(context.Background(), newRootCommand(), os.Args[1:], os.Stderr))
}

// execute runs cmd on args and returns the process exit status. Every error is printed here, as
// `photon: <err>` on stderr, so a failure reads the same whichever subcommand produced it; Cobra
// only prints the usage text, and only for a command line it refused.
func execute(ctx context.Context, cmd *cobra.Command, args []string, stderr io.Writer) int {
	cmd.SetArgs(args)
	cmd.SetErr(stderr)
	err := cmd.ExecuteContext(ctx)
	if err != nil {
		//nolint:errcheck // stderr is where a failure is reported; a failed write there has nowhere left to go.
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
	}
	return exitCodeFor(err)
}
