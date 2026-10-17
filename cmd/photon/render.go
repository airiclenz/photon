package main

import (
	"github.com/spf13/cobra"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/render"
)

// newRenderCommand renders the shipped GIF from a take, the storyboard deciding every section's
// length, hold, zoom and cut.
func newRenderCommand() *cobra.Command {
	var configPath, workPath string
	var options render.Options
	cmd := &cobra.Command{
		Use:   "render <storyboard> [<take>]",
		Short: "Render the shipped GIF from a take, framed as the storyboard says",
		Long: "render lays each beat of the take onto its section's duration — the hold at 1×, the\n" +
			"rest sped up to fit or frozen on its last frame — rasterizes every frame at the\n" +
			"storyboard's frame, composes the zooms and the click cursor, and streams the frames into\n" +
			"ffmpeg for a per-clip palette encode, then shrinks the GIF with gifsicle -O3 when that is\n" +
			"on PATH. It prints the GIF's path, size and length and one speed row per beat.\n" +
			"The take defaults to <work>/<clip>.take; the work dir is --work, else $" + clip.WorkEnv + ",\n" +
			"else the config's work, else ~/.cache/<name>-photon. The GIF defaults to the storyboard's\n" +
			"ship path. --dry-run prints the ffmpeg command line instead of rendering.",
		Args: cobra.RangeArgs(1, 2),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			loaded, err := clip.Load(args[0], configPath, workPath)
			if err != nil {
				return err
			}
			if len(args) > 1 {
				options.Take = args[1]
			}
			return render.Run(cmd.Context(), loaded, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
		}),
	}
	cmd.Flags().StringVarP(&options.Out, "output", "o", "", "the GIF to write (default: the storyboard's ship path)")
	cmd.Flags().BoolVar(&options.DryRun, "dry-run", false, "print the ffmpeg command line and exit without rendering")
	cmd.Flags().StringVar(&configPath, configFlag, "", "the photon config to use instead of the nearest photon.yaml")
	cmd.Flags().StringVar(&workPath, workFlag, "", "the rig's work dir, where the take is read from")
	return cmd
}
