package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/render"
)

// watchFlag names render's flag that renders again on each change to the clip's files.
const watchFlag = "watch"

// newRenderCommand renders the shipped GIF from a take, the storyboard deciding every section's
// length, hold and cut.
func newRenderCommand() *cobra.Command {
	var configPath, workPath string
	var isWatching bool
	var options render.Options
	cmd := &cobra.Command{
		Use:   "render <storyboard> [<take>]",
		Short: "Render the shipped GIF from a take, framed as the storyboard says",
		Long: "render lays each beat of the take onto its section's duration — the hold at 1×, " +
			"the\nrest sped up to fit or frozen on its last frame — rasterizes every frame at " +
			"the\nstoryboard's frame, composes the camera, following its positions and " +
			"zooms,\nand the cursor over the clicks and drags, and streams the frames through " +
			"ffmpeg twice\nfor a per-clip palette encode: pass 1 writes the palette to\n" +
			"<work>/<clip>.palette.png, pass 2 maps the frames onto it into the GIF, and\n" +
			"the palette is removed after. It then shrinks the GIF " +
			"with gifsicle -O3 when that is on\nPATH, and prints the GIF's path, size and length " +
			"and one speed row per beat.\nThe take defaults to <work>/<clip>.take; the work dir " +
			"is --work, else $" + clip.WorkEnv + ",\nelse the config's work, else " +
			"~/.cache/<name>-photon. The GIF defaults to the storyboard's\nship path. --dry-run " +
			"prints both ffmpeg command lines instead of rendering.\n--watch renders once, then " +
			"again each time the storyboard, config, take or a caption\nfont file changes; a " +
			"failed load or render prints its error and the watch goes on.\nExit status: 0 when " +
			"the GIF was written, or when --watch is stopped with Ctrl-C\nor SIGTERM; 1 when the " +
			"take could not be rendered; 2 for a bad command line.",
		Args: cobra.RangeArgs(1, 2),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				options.Take = args[1]
			}
			if isWatching {
				ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
				defer stop()
				inputs := render.WatchInputs{
					Storyboard: args[0], ConfigFlag: configPath, WorkFlag: workPath,
				}
				return render.Watch(ctx, inputs, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
			}
			loaded, err := clip.Load(args[0], configPath, workPath)
			if err != nil {
				return err
			}
			return render.Run(cmd.Context(), loaded, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
		}),
	}
	cmd.Flags().StringVarP(&options.Out, "output", "o", "",
		"the GIF to write (default: the storyboard's ship path)")
	cmd.Flags().BoolVar(&options.DryRun, "dry-run", false,
		"print both ffmpeg command lines, palette pass first, and exit without rendering")
	cmd.Flags().BoolVar(&isWatching, watchFlag, false,
		"render again each time the storyboard, config, take or a caption font file changes")
	cmd.Flags().StringVar(&configPath, configFlag, "",
		"the photon config to use instead of the nearest photon.yaml")
	cmd.Flags().StringVar(&workPath, workFlag, "",
		"the rig's work dir, where the take is read and the palette written")
	return cmd
}
