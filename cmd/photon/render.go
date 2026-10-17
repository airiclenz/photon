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
			"storyboard's frame, composes the zooms and the cursor over the clicks and drags, and\n" +
			"streams the frames through ffmpeg twice for a per-clip palette encode: pass 1 writes the\n" +
			"palette to <work>/<clip>.palette.png, pass 2 maps the frames onto it into the GIF, and\n" +
			"the palette is removed after. It then shrinks the GIF with gifsicle -O3 when that is on\n" +
			"PATH, and prints the GIF's path, size and length and one speed row per beat.\n" +
			"The take defaults to <work>/<clip>.take; the work dir is --work, else $" + clip.WorkEnv + ",\n" +
			"else the config's work, else ~/.cache/<name>-photon. The GIF defaults to the storyboard's\n" +
			"ship path. --dry-run prints both ffmpeg command lines instead of rendering.\n" +
			"Exit status: 0 when the GIF was written; 1 when the take could not be rendered;\n" +
			"2 for a bad command line.",
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
	cmd.Flags().BoolVar(&options.DryRun, "dry-run", false, "print both ffmpeg command lines, palette pass first, and exit without rendering")
	cmd.Flags().StringVar(&configPath, configFlag, "", "the photon config to use instead of the nearest photon.yaml")
	cmd.Flags().StringVar(&workPath, workFlag, "", "the rig's work dir, where the take is read and the palette written")
	return cmd
}
