package lerobot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ioai-tech/lerobot-go/internal/formatcheck"
	"github.com/ioai-tech/lerobot-go/internal/meta"
)

// ValidationReport is the result of layout validation.
type ValidationReport struct {
	OK       bool
	Errors   []string
	Warnings []string
	Info     meta.DatasetInfo
	Summary  string
	Version  string
}

// SchemaDiffReport compares meta/info.json between two datasets.
type SchemaDiffReport struct {
	OK     bool
	Diffs  []string
	Errors []string
}

// Inspector validates LeRobot datasets and compares schemas.
type Inspector interface {
	Validate(ctx context.Context, root string) (*ValidationReport, error)
	ValidateStrict(ctx context.Context, root string) (*ValidationReport, error)
	SchemaDiff(ctx context.Context, goldenRoot, candidateRoot string) (*SchemaDiffReport, error)
}

type defaultInspector struct{}

// NewInspector returns a dataset layout inspector.
func NewInspector() Inspector { return &defaultInspector{} }

func (d *defaultInspector) Validate(ctx context.Context, root string) (*ValidationReport, error) {
	return d.validate(ctx, root, false)
}

func (d *defaultInspector) ValidateStrict(ctx context.Context, root string) (*ValidationReport, error) {
	return d.validate(ctx, root, true)
}

func (d *defaultInspector) validate(ctx context.Context, root string, strict bool) (*ValidationReport, error) {
	_ = ctx
	report := &ValidationReport{OK: true}
	fc, err := formatcheck.ValidateWithOptions(root, formatcheck.Options{Strict: strict})
	if err != nil {
		return nil, err
	}
	report.Warnings = append(report.Warnings, fc.Warnings...)
	report.Summary = fc.Summary
	report.Version = fc.Version
	if !fc.OK {
		report.OK = false
		report.Errors = append(report.Errors, fc.Errors...)
	}
	info, err := meta.LoadInfo(root)
	if err != nil {
		report.OK = false
		report.Errors = append(report.Errors, err.Error())
		return report, nil
	}
	report.Info = info
	if _, err := os.Stat(filepath.Join(root, meta.StatsPath)); err != nil {
		report.Warnings = append(report.Warnings, "missing stats.json")
	}
	return report, nil
}

func (d *defaultInspector) SchemaDiff(ctx context.Context, goldenRoot, candidateRoot string) (*SchemaDiffReport, error) {
	_ = ctx
	golden, err := meta.LoadInfo(goldenRoot)
	if err != nil {
		return nil, err
	}
	candidate, err := meta.LoadInfo(candidateRoot)
	if err != nil {
		return nil, err
	}
	report := &SchemaDiffReport{OK: true}
	if golden.CodebaseVersion != candidate.CodebaseVersion {
		report.Diffs = append(report.Diffs, fmt.Sprintf("codebase_version: %s vs %s", golden.CodebaseVersion, candidate.CodebaseVersion))
	}
	if golden.FPS != candidate.FPS {
		report.Diffs = append(report.Diffs, fmt.Sprintf("fps: %d vs %d", golden.FPS, candidate.FPS))
	}
	for k, gf := range golden.Features {
		cf, ok := candidate.Features[k]
		if !ok {
			report.Diffs = append(report.Diffs, fmt.Sprintf("missing feature %q in candidate", k))
			continue
		}
		if !gf.Equal(cf) {
			report.Diffs = append(report.Diffs, fmt.Sprintf("feature %q metadata differs", k))
		}
	}
	for k := range candidate.Features {
		if _, ok := golden.Features[k]; !ok {
			report.Diffs = append(report.Diffs, fmt.Sprintf("extra feature %q in candidate", k))
		}
	}
	if len(report.Diffs) > 0 {
		report.OK = false
	}
	return report, nil
}
