package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ioai-tech/lerobot-go/internal/formatcheck"
	"github.com/ioai-tech/lerobot-go/internal/manifest"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/stats"
	"github.com/ioai-tech/lerobot-go/lerobot"
	"github.com/spf13/cobra"
)

func NewCreateCmd() *cobra.Command {
	var output, version, staging, featuresPath, robotType, statsMode string
	var fps int
	var force bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Finalize episode staging into a LeRobot dataset",
		Long: `Merge completed staging episodes (ep_*) into the final on-disk dataset layout.

Use this after parallel episode writers have finished under <output>/_staging.

Examples:
  lerobot-go create -o ./dataset --version v3.0 --fps 30 --features ./features.json
  lerobot-go create -o ./dataset --version v2.1 --fps 10 --features ./features.json --stats-mode full`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := CleanPath(output)
			if err != nil {
				return fmt.Errorf("create: %w", err)
			}
			if err := EnsureOutputDir(out, force); err != nil {
				return fmt.Errorf("create: output: %w", err)
			}
			ver, err := ParseVersion(version)
			if err != nil {
				return fmt.Errorf("create: --version: %w", err)
			}
			statsOpts, err := ParseStatsMode(statsMode)
			if err != nil {
				return fmt.Errorf("create: %w", err)
			}
			features, err := loadFeaturesFile(featuresPath)
			if err != nil {
				return fmt.Errorf("create: --features: %w", err)
			}
			stagingRoot := staging
			if stagingRoot == "" {
				stagingRoot = filepath.Join(out, "_staging")
			} else {
				stagingRoot, err = CleanPath(stagingRoot)
				if err != nil {
					return fmt.Errorf("create: --staging: %w", err)
				}
			}
			dirs, err := manifest.ListStagingEpisodes(stagingRoot)
			if err != nil {
				return fmt.Errorf("create: staging: %w", err)
			}
			if len(dirs) == 0 {
				return fmt.Errorf("create: no completed episodes in %s", stagingRoot)
			}
			if fps <= 0 {
				return fmt.Errorf("create: --fps must be positive")
			}
			Logf(Globals, "create finalize %d episodes -> %s (%s)", len(dirs), out, VersionString(ver))
			ctx := context.Background()
			if err := lerobot.Merge(ctx, lerobot.MergeConfig{
				Version: ver, StagingRoot: stagingRoot, OutputRoot: out,
				RobotType: robotType, FPS: fps, Features: features,
				FFmpeg: lerobot.FFmpegConfig{FFmpegPath: Globals.FFmpegPath},
				Stats:  statsModeFromOpts(statsOpts),
			}); err != nil {
				return fmt.Errorf("create: %w", err)
			}
			post, err := formatcheck.Validate(out)
			if err != nil {
				return err
			}
			if !post.OK {
				return fmt.Errorf("create: output validation failed: %v", post.Errors)
			}
			if Globals.JSON {
				return PrintJSON(map[string]any{
					"ok": true, "output": out, "version": VersionString(ver),
					"episodes": len(dirs), "stats_mode": statsModeLabel(statsOpts), "summary": post.Summary,
				})
			}
			fmt.Printf("OK: created dataset at %s (%s, %d episodes)\n", out, VersionString(ver), len(dirs))
			fmt.Println(post.Summary)
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "dataset root (final layout written here)")
	cmd.Flags().StringVar(&version, "version", "", "dataset version: v2.1 or v3.0")
	cmd.Flags().StringVar(&staging, "staging", "", "staging root with ep_* dirs (default: <output>/_staging)")
	cmd.Flags().StringVar(&featuresPath, "features", "", "JSON file: map of feature name to {dtype, shape}")
	cmd.Flags().IntVar(&fps, "fps", 0, "dataset FPS")
	cmd.Flags().StringVar(&robotType, "robot-type", "", "robot type stored in meta/info.json")
	cmd.Flags().StringVar(&statsMode, "stats-mode", "sampled", "image/video stats: sampled (official default) or full")
	cmd.Flags().BoolVar(&force, "force", false, "allow non-empty output directory")
	_ = cmd.MarkFlagRequired("output")
	_ = cmd.MarkFlagRequired("version")
	_ = cmd.MarkFlagRequired("features")
	_ = cmd.MarkFlagRequired("fps")
	return cmd
}

func loadFeaturesFile(path string) (map[string]meta.FeatureSpec, error) {
	clean, err := CleanPath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return nil, err
	}
	var features map[string]meta.FeatureSpec
	if err := json.Unmarshal(data, &features); err != nil {
		return nil, err
	}
	if len(features) == 0 {
		return nil, fmt.Errorf("empty features map")
	}
	return features, nil
}

func statsModeLabel(opts stats.Options) string {
	if opts.Mode == stats.ModeFull {
		return "full"
	}
	return "sampled"
}
