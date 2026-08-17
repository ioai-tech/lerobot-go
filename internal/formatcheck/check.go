package formatcheck

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	"github.com/ioai-tech/lerobot-go/internal/stats"
)

var requiredDataColumns = []string{
	"timestamp", "frame_index", "episode_index", "index", "task_index",
}

type Result struct {
	OK       bool
	Version  string
	Errors   []string
	Warnings []string
	Files    []string
	Summary  string
}

func Validate(root string) (Result, error) {
	return ValidateWithOptions(root, Options{})
}

func ValidateWithOptions(root string, opts Options) (Result, error) {
	info, err := meta.LoadInfo(root)
	if err != nil {
		return Result{OK: false, Errors: []string{err.Error()}}, nil
	}
	if opts.Strict && !hasDataDir(root) {
		return Result{
			OK: false, Version: info.CodebaseVersion,
			Errors: []string{"strict: missing data/ directory"},
		}, nil
	}
	switch info.CodebaseVersion {
	case meta.CodebaseV21:
		return validateV21(root, info, opts), nil
	case meta.CodebaseV30:
		return validateV30(root, info, opts), nil
	default:
		return Result{
			OK:      false,
			Version: info.CodebaseVersion,
			Errors:  []string{fmt.Sprintf("unsupported codebase_version %q", info.CodebaseVersion)},
		}, nil
	}
}

func validateV21(root string, info meta.DatasetInfo, opts Options) Result {
	r := Result{OK: true, Version: meta.CodebaseV21}
	r.Files = listFiles(root)

	check := func(err string) {
		r.OK = false
		r.Errors = append(r.Errors, err)
	}
	warn := func(msg string) {
		r.Warnings = append(r.Warnings, msg)
	}

	if !strings.HasPrefix(info.DataPath, meta.DataDir+"/") {
		check(fmt.Sprintf("v2.1 data_path unexpected: %s", info.DataPath))
	}
	for _, rel := range []string{
		meta.InfoPath, meta.LegacyEpisodesPath,
		meta.LegacyTasksPath, meta.LegacyEpisodesStatsPath,
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			check("missing " + rel)
		}
	}
	if _, err := os.Stat(filepath.Join(root, meta.StatsPath)); err != nil {
		warnMissingStats(opts.Strict, warn)
	} else {
		checkStatsVisualCoverage(root, info, check)
	}

	episodes, err := readJSONL(filepath.Join(root, meta.LegacyEpisodesPath))
	if err != nil {
		check("episodes.jsonl: " + err.Error())
	} else if len(episodes) != info.TotalEpisodes {
		check(fmt.Sprintf("episodes.jsonl rows %d != total_episodes %d", len(episodes), info.TotalEpisodes))
	}

	tasks, err := readJSONL(filepath.Join(root, meta.LegacyTasksPath))
	if err != nil {
		check("tasks.jsonl: " + err.Error())
	} else if len(tasks) != info.TotalTasks {
		check(fmt.Sprintf("tasks.jsonl rows %d != total_tasks %d", len(tasks), info.TotalTasks))
	}

	statsRows, err := readJSONL(filepath.Join(root, meta.LegacyEpisodesStatsPath))
	if err != nil {
		check("episodes_stats.jsonl: " + err.Error())
	} else if len(statsRows) != info.TotalEpisodes {
		check(fmt.Sprintf("episodes_stats.jsonl rows %d != total_episodes %d", len(statsRows), info.TotalEpisodes))
	} else {
		for _, row := range statsRows {
			statsObj, ok := row["stats"].(map[string]any)
			if !ok {
				check("episodes_stats.jsonl: stats must be a feature map, not nested episode keys")
				break
			}
			if len(statsObj) == 0 {
				warn("episodes_stats.jsonl: empty stats for an episode")
			}
		}
	}

	featureCols := expectedDataFeatureColumns(info)
	var frameSum int
	ctx := context.Background()
	var globalExpect int64
	dataPaths := v21EpisodeDataPaths(root, info)
	if len(dataPaths) != info.TotalEpisodes {
		check(fmt.Sprintf("data parquet files %d != total_episodes %d", len(dataPaths), info.TotalEpisodes))
	}
	for ep, rel := range dataPaths {
		pq := filepath.Join(root, rel)
		if _, err := os.Stat(pq); err != nil {
			check(fmt.Sprintf("missing %s", rel))
			continue
		}
		rows, err := parquetx.TableNumRows(pq)
		if err != nil {
			check(err.Error())
			continue
		}
		frameSum += int(rows)
		tbl, err := parquetx.ReadTable(ctx, pq, nil)
		if err != nil {
			check(err.Error())
			continue
		}
		if err := checkParquetColumns(tbl, requiredDataColumns); err != nil {
			check(rel + ": " + err.Error())
		}
		checkFeatureColumns(tbl, featureCols, rel, check)
		if opts.Strict {
			checkTimestamps(tbl, info.FPS, rel, check, warn)
		}
		indices, err := parquetx.ExtractInt64Column(tbl, "index")
		tbl.Release()
		if err == nil && len(indices) > 0 {
			if indices[0] != globalExpect {
				check(fmt.Sprintf("episode %d: index starts at %d, want %d", ep, indices[0], globalExpect))
			}
			for i, idx := range indices {
				if idx != globalExpect+int64(i) {
					check(fmt.Sprintf("episode %d: index not contiguous at row %d", ep, i))
					break
				}
			}
			globalExpect += int64(len(indices))
		}
	}
	if frameSum != info.TotalFrames {
		check(fmt.Sprintf("parquet frame sum %d != total_frames %d", frameSum, info.TotalFrames))
	}

	wantSplit := fmt.Sprintf("0:%d", info.TotalEpisodes)
	if split, ok := info.Splits["train"]; !ok || split != wantSplit {
		check(fmt.Sprintf("splits.train want %q got %q", wantSplit, split))
	}
	if opts.Strict {
		checkVideosV21(root, info, check)
	}

	r.Summary = fmt.Sprintf("v2.1: %d episodes, %d frames, %d tasks, %d files",
		info.TotalEpisodes, info.TotalFrames, info.TotalTasks, len(r.Files))
	return r
}

func validateV30(root string, info meta.DatasetInfo, opts Options) Result {
	r := Result{OK: true, Version: meta.CodebaseV30}
	r.Files = listFiles(root)

	check := func(err string) {
		r.OK = false
		r.Errors = append(r.Errors, err)
	}
	warn := func(msg string) {
		r.Warnings = append(r.Warnings, msg)
	}

	if _, err := os.Stat(filepath.Join(root, meta.StatsPath)); err != nil {
		warnMissingStats(opts.Strict, warn)
	} else {
		checkStatsVisualCoverage(root, info, check)
	}

	if !strings.Contains(info.DataPath, "file-{file_index") {
		check(fmt.Sprintf("v3.0 data_path unexpected: %s", info.DataPath))
	}
	for _, rel := range []string{meta.InfoPath, meta.StatsPath, meta.DefaultTasksPath} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			check("missing " + rel)
		}
	}

	epFiles, _ := filepath.Glob(filepath.Join(root, meta.EpisodesDir, "chunk-*", "file-*.parquet"))
	if len(epFiles) == 0 {
		check("missing meta/episodes/*.parquet")
	}
	dataFiles, _ := filepath.Glob(filepath.Join(root, meta.DataDir, "chunk-*", "file-*.parquet"))
	if len(dataFiles) == 0 {
		check("missing data/chunk-*/file-*.parquet")
	}

	ctx := context.Background()
	featureCols := expectedDataFeatureColumns(info)
	episodesRows, err := parquetx.ReadEpisodesMeta(root)
	if err != nil {
		check("episodes meta: " + err.Error())
	} else {
		tableCache := map[string]arrow.Table{}
		defer func() {
			for _, tbl := range tableCache {
				tbl.Release()
			}
		}()
		validatedFiles := map[string]bool{}
		for _, ep := range episodesRows {
			rel := meta.DataPath(int(ep.DataChunkIndex), int(ep.DataFileIndex))
			pq := filepath.Join(root, rel)
			tbl, err := cachedValidationTable(ctx, tableCache, pq)
			if err != nil {
				check(err.Error())
				continue
			}
			if !validatedFiles[rel] {
				if err := checkParquetColumns(tbl, requiredDataColumns); err != nil {
					check(rel + ": " + err.Error())
				}
				checkFeatureColumns(tbl, featureCols, rel, check)
				if opts.Strict {
					checkTimestamps(tbl, info.FPS, rel, check, warn)
				}
				validatedFiles[rel] = true
			}
			slice, err := parquetx.SliceTableByIndexRange(tbl, ep.DatasetFromIndex, ep.DatasetToIndex)
			if err != nil {
				check(fmt.Sprintf("episode %d: %v", ep.EpisodeIndex, err))
				continue
			}
			if slice.NumRows() != ep.Length {
				check(fmt.Sprintf("episode %d: slice rows %d != length %d", ep.EpisodeIndex, slice.NumRows(), ep.Length))
			}
			slice.Release()
		}
	}
	checkTasksParquetV30(ctx, root, info, check)
	checkEpisodeMetaV30(root, info, check)
	if opts.Strict {
		checkVideosV30(root, info, check, warn)
	}

	var epMetaRows int64
	for _, pq := range epFiles {
		n, err := parquetx.TableNumRows(pq)
		if err != nil {
			check(err.Error())
			continue
		}
		epMetaRows += n
	}
	if int(epMetaRows) != info.TotalEpisodes {
		check(fmt.Sprintf("episodes meta rows %d != total_episodes %d", epMetaRows, info.TotalEpisodes))
	}

	wantSplit := fmt.Sprintf("0:%d", info.TotalEpisodes)
	if split, ok := info.Splits["train"]; !ok || split != wantSplit {
		check(fmt.Sprintf("splits.train want %q got %q", wantSplit, split))
	}

	r.Summary = fmt.Sprintf("v3.0: %d episodes, %d frames, %d tasks, %d data files, %d meta episode files, %d total files",
		info.TotalEpisodes, info.TotalFrames, info.TotalTasks, len(dataFiles), len(epFiles), len(r.Files))
	return r
}

func checkParquetColumns(tbl arrow.Table, cols []string) error {
	schema := tbl.Schema()
	for _, col := range cols {
		if len(schema.FieldIndices(col)) == 0 {
			return fmt.Errorf("missing column %q", col)
		}
	}
	return nil
}

func readJSONL(path string) ([]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var rows []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, sc.Err()
}

func checkStatsVisualCoverage(root string, info meta.DatasetInfo, check func(string)) {
	desc := make(map[string]stats.FeatureDesc, len(info.Features))
	for k, v := range info.Features {
		desc[k] = stats.FeatureDesc{DType: v.DType, Shape: v.Shape}
	}
	if err := stats.CheckStatsFileVisualCoverage(filepath.Join(root, meta.StatsPath), desc); err != nil {
		check(err.Error())
	}
}

func listFiles(root string) []string {
	var files []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		files = append(files, rel)
		return nil
	})
	return files
}
