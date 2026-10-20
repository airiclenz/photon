package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/airiclenz/photon"
	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/tune"
)

// portFlag is the flag naming the port the tuning page is served on.
const portFlag = "port"

// newTuneCommand serves the timing editor: a page on this machine that previews a take, in draft
// frames while playing or scrubbing and settled on the frame `photon render` composes once
// stopped, and saves timing, camera position, zoom, caption and frame edits back into the
// storyboard.
func newTuneCommand() *cobra.Command {
	var configPath, workPath string
	var options tune.Options
	cmd := &cobra.Command{
		Use:   "tune <storyboard> [<take>]",
		Short: "Tune a storyboard's timing, camera positions and zooms, and frame against a take",
		Long: "tune serves a page on 127.0.0.1 that previews the take: playing or scrubbing\n" +
			"shows draft frames, drawn from a raster at the shipped size with a fast\n" +
			"scaler, and the preview settles on the exact frame render composes once\n" +
			"stopped. It edits each beat's duration, hold and cut on a timeline and the\n" +
			"clip's frame.width, fps, font_size, line_height, padding and scale. Lanes\n" +
			"under the beats show the camera's positions with the transitions between them,\n" +
			"its zooms and the captions: a click opens one in the inspector, a drag moves it\n" +
			"and a lane's + adds one. A selected position shows the full frame on the paused\n" +
			"preview, with the camera's crop and a marker to drag onto the cell it aims at.\n" +
			"Save writes the edits back into the storyboard in place, changing only the\n" +
			"edited values; it is refused while an edit leaves the storyboard invalid or the\n" +
			"file changed on disk since it was loaded. A target picker on the paused preview\n" +
			"proposes a target the resolver finds on the picked cells, ready to set as the\n" +
			"selected position's text target.\npause and the do actions show read-only: " +
			"changing them needs photon record.\n" +
			"tune listens on --port (default: any free port), prints the page's " +
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
			options.Build = photon.Build()
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
