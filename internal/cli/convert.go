package cli

import (
	"context"
	"fmt"

	"github.com/ioai-tech/lerobot-go/internal/convert"
	"github.com/ioai-tech/lerobot-go/internal/formatcheck"
	"github.com/ioai-tech/lerobot-go/internal/video"
	"github.com/ioai-tech/lerobot-go/lerobot"
	"github.com/spf13/cobra"
)

func NewConvertCmd() *cobra.Command {
	var input, output, fromVer, toVer string
	var force bool
	var dataMB, videoMB, chunks int
	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert LeRobot dataset between v2.1 and v3.0",
		Long: `Convert a LeRobot dataset from v2.1 to v3.0 or vice versa.

Examples:
  lerobot-go convert -i ./dataset_v21 -o ./dataset_v30 --to v3.0
  lerobot-go convert -i ./dataset_v30 -o ./dataset_v21 --to v2.1`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			in, err := CleanPath(input)
			if err != nil {
				return fmt.Errorf("convert: %w", err)
			}
			out, err := CleanPath(output)
			if err != nil {
				return fmt.Errorf("convert: %w", err)
			}
			if err := EnsureDatasetRoot(in); err != nil {
				return fmt.Errorf("convert: input: %w", err)
			}
			if err := EnsureOutputDir(out, force); err != nil {
				return fmt.Errorf("convert: output: %w", err)
			}
			to, err := ParseVersion(toVer)
			if err != nil {
				return fmt.Errorf("convert: --to: %w", err)
			}
			var from lerobot.Version
			if fromVer != "" {
				from, err = ParseVersion(fromVer)
				if err != nil {
					return fmt.Errorf("convert: --from: %w", err)
				}
			} else {
				from, err = LoadVersionFromRoot(in)
				if err != nil {
					return fmt.Errorf("convert: %w", err)
				}
			}
			if from == to {
				return fmt.Errorf("convert: --from and --to must differ")
			}
			if err := formatcheck.ValidateStrict(in); err != nil {
				return fmt.Errorf("convert: input preflight: %w", err)
			}
			preflight, err := formatcheck.Validate(in)
			if err != nil {
				return err
			}
			if !preflight.OK {
				return fmt.Errorf("convert: input dataset invalid")
			}
			locator := video.NewLocator(video.Config{FFmpegPath: Globals.FFmpegPath})
			cfg := convert.Config{
				Input: in, Output: out,
				From: from, To: to,
				DataFileSizeMB: dataMB, VideoFileSizeMB: videoMB, ChunksSize: chunks,
				Locator: locator,
			}
			Logf(Globals, "convert %s -> %s", VersionString(from), VersionString(to))
			ctx := context.Background()
			if err := convert.Run(ctx, cfg); err != nil {
				return fmt.Errorf("convert: %w", err)
			}
			post, err := formatcheck.Validate(out)
			if err != nil {
				return err
			}
			if !post.OK {
				return fmt.Errorf("convert: output validation failed: %v", post.Errors)
			}
			if Globals.JSON {
				return PrintJSON(map[string]any{
					"ok": true, "input": in, "output": out,
					"from": VersionString(from), "to": VersionString(to),
					"summary": post.Summary,
				})
			}
			fmt.Printf("OK: converted %s -> %s at %s\n", VersionString(from), VersionString(to), out)
			fmt.Println(post.Summary)
			return nil
		},
	}
	cmd.Flags().StringVarP(&input, "input", "i", "", "source dataset root")
	cmd.Flags().StringVarP(&output, "output", "o", "", "destination dataset root")
	cmd.Flags().StringVar(&toVer, "to", "", "target version: v2.1 or v3.0")
	cmd.Flags().StringVar(&fromVer, "from", "", "source version (default: read from input meta/info.json)")
	cmd.Flags().BoolVar(&force, "force", false, "allow non-empty output directory")
	cmd.Flags().IntVar(&dataMB, "data-file-size-mb", 100, "v3.0 data chunk size limit (uncompressed MB)")
	cmd.Flags().IntVar(&videoMB, "video-file-size-mb", 200, "v3.0 video chunk size limit (MB)")
	cmd.Flags().IntVar(&chunks, "chunks-size", 1000, "max files per chunk")
	_ = cmd.MarkFlagRequired("input")
	_ = cmd.MarkFlagRequired("output")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}
