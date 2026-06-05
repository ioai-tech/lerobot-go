package cli

import (
	"context"
	"fmt"

	"github.com/ioai-tech/lerobot-go/internal/aggregate"
	"github.com/ioai-tech/lerobot-go/internal/formatcheck"
	"github.com/ioai-tech/lerobot-go/internal/video"
	"github.com/spf13/cobra"
)

func NewMergeCmd() *cobra.Command {
	var output, toVer, statsMode string
	var inputs []string
	var force bool
	var dataMB, videoMB, chunks int
	cmd := &cobra.Command{
		Use:   "merge [datasets...]",
		Short: "Merge multiple LeRobot datasets into one",
		Long: `Merge two or more completed LeRobot datasets into a single dataset.

Examples:
  lerobot-go merge -o ./merged --to v3.0 -i ./a -i ./b
  lerobot-go merge -o ./merged_v21 --to v2.1 ./ds1 ./ds2`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := append(append([]string{}, inputs...), args...)
			paths, err := DedupePaths(paths)
			if err != nil {
				return fmt.Errorf("merge: %w", err)
			}
			if len(paths) < 2 {
				return fmt.Errorf("merge: at least two input datasets required")
			}
			out, err := CleanPath(output)
			if err != nil {
				return fmt.Errorf("merge: %w", err)
			}
			if err := EnsureOutputDir(out, force); err != nil {
				return fmt.Errorf("merge: output: %w", err)
			}
			to, err := ParseVersion(toVer)
			if err != nil {
				return fmt.Errorf("merge: --to: %w", err)
			}
			for _, p := range paths {
				if err := EnsureDatasetRoot(p); err != nil {
					return fmt.Errorf("merge: input %q: %w", p, err)
				}
				if err := formatcheck.ValidateStrict(p); err != nil {
					return fmt.Errorf("merge: input %q: %w", p, err)
				}
				rep, err := formatcheck.Validate(p)
				if err != nil {
					return err
				}
				if !rep.OK {
					return fmt.Errorf("merge: input %q invalid: %v", p, rep.Errors)
				}
			}
			statsOpts, err := ParseStatsMode(statsMode)
			if err != nil {
				return fmt.Errorf("merge: %w", err)
			}
			locator := video.NewLocator(video.Config{FFmpegPath: Globals.FFmpegPath})
			cfg := aggregate.Config{
				Inputs: paths, Output: out, To: to,
				DataFileSizeMB: dataMB, VideoFileSizeMB: videoMB, ChunksSize: chunks,
				Locator: locator, Stats: statsOpts,
			}
			Logf(Globals, "merge %d datasets -> %s (%s)", len(paths), out, VersionString(to))
			ctx := context.Background()
			if err := aggregate.Run(ctx, cfg); err != nil {
				return fmt.Errorf("merge: %w", err)
			}
			post, err := formatcheck.Validate(out)
			if err != nil {
				return err
			}
			if !post.OK {
				return fmt.Errorf("merge: output validation failed: %v", post.Errors)
			}
			if Globals.JSON {
				return PrintJSON(map[string]any{
					"ok": true, "output": out, "to": VersionString(to),
					"inputs": paths, "summary": post.Summary,
				})
			}
			fmt.Printf("OK: merged %d datasets -> %s (%s)\n", len(paths), out, VersionString(to))
			fmt.Println(post.Summary)
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output dataset root")
	cmd.Flags().StringVar(&toVer, "to", "", "output version: v2.1 or v3.0")
	cmd.Flags().StringArrayVarP(&inputs, "input", "i", nil, "source dataset root (repeatable)")
	cmd.Flags().BoolVar(&force, "force", false, "allow non-empty output directory")
	cmd.Flags().IntVar(&dataMB, "data-file-size-mb", 100, "v3.0 data chunk size limit (uncompressed MB)")
	cmd.Flags().IntVar(&videoMB, "video-file-size-mb", 200, "v3.0 video chunk size limit (MB)")
	cmd.Flags().IntVar(&chunks, "chunks-size", 1000, "max files per chunk")
	cmd.Flags().StringVar(&statsMode, "stats-mode", "sampled", "image/video stats when recomputing from media (multi-dataset merge keeps source stats)")
	_ = cmd.MarkFlagRequired("output")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}
