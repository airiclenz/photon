package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/tune"
)

// portFlag is the flag naming the port the tuning page is served on.
const portFlag = "port"

// newTuneCommand serves the timing editor: a page on this machine that previews a take as
// `photon render` composes it and saves timing, zoom and frame edits back into the storyboard.
func newTuneCommand() *cobra.Command {
	var configPath, workPath string
	var options tune.Options
	cmd := &cobra.Command{
		Use:   "tune <storyboard> [<take>]",
		Short: "Tune a storyboard's timing, zoom and frame against a take in a local page",
		Long: "tune serves a page on 127.0.0.1 that previews the take frame for frame as render\n" +
			"composes it, and edits each beat's duration, hold, cut and zoom and the clip's\n" +
			"frame.width and frame.fps on a timeline. Save writes the edits back into the\n" +
			"storyboard in place, changing only the edited values; it is refused while an edit\n" +
			"leaves the storyboard invalid or the file changed on disk since it was loaded. A " +
			"target\npicker on the paused preview proposes a target the resolver finds on the " +
			"picked cells.\npause and the do actions show read-only: changing them needs photon " +
			"record.\ntune listens on --port (default: any free port), prints the page's " +
			"address, and runs\nuntil Ctrl-C. It never opens a browser. The take defaults to " +
			"<work>/<clip>.take; the work\ndir is --work, else $" + clip.WorkEnv + ", else the " +
			"config's work, else ~/.cache/<name>-photon.\nExit status: 0 when stopped with " +
			"Ctrl-C; 1 when the take could not be tuned or the page\nnot served; 2 for a bad " +
			"command line.",
		Args: cobra.RangeArgs(1, 2),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			loaded, err := clip.Load(args[0], configPath, workPath)
			if err != nil {
				return err
			}
			if len(args) > 1 {
				options.Take = args[1]
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return tune.Run(ctx, loaded, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
		}),
	}
	cmd.Flags().IntVar(&options.Port, portFlag, 0,
		"the port to serve the page on, on 127.0.0.1 (default: any free port)")
	cmd.Flags().StringVar(&configPath, configFlag, "",
		"the photon config to use instead of the nearest photon.yaml")
	cmd.Flags().StringVar(&workPath, workFlag, "", "the rig's work dir, where the take is read")
	return cmd
}
