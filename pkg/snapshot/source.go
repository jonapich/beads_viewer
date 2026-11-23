package snapshot

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Dicklesworthstone/beads_viewer/internal/datasource"
	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/loader"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/recipe"
)

// Source is the raw result of reading a tracker file or database: the visible
// issues plus the authority (readiness index and hash) computed over the full
// record set, including tombstones and recipe-filtered rows.
type Source struct {
	loader.PooledIssues
	Authority         *model.ReadinessIndex
	AuthorityHash     string
	AuthorityComplete bool
}

func isSQLitePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".db", ".sqlite", ".sqlite3":
		return true
	default:
		return false
	}
}

func openSQLiteContext(ctx context.Context, path string) (*datasource.SQLiteReader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source, ok, err := datasource.SourceFromFile(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("unsupported SQLite source path: %s", path)
	}
	return datasource.NewSQLiteReaderContext(ctx, source)
}

// LoadSourceSync reads path without a context. degraded marks the source as
// incomplete authority (for example after a recovered read error).
//
// Authority, AuthorityHash and AuthorityComplete cover every record, including
// tombstones and rows the filter rejects. Issues has tombstones removed and
// opts.IssueFilter applied. JSONL rows come from the loader pool, so return
// Source.PoolRefs with loader.ReturnIssuePtrsToPool once Issues is no longer
// used, or leave them to the GC.
func LoadSourceSync(path string, opts loader.ParseOptions, degraded bool) (Source, error) {
	var loaded loader.PooledIssues
	var err error
	complete := true
	switch strings.ToLower(filepath.Ext(path)) {
	case ".db", ".sqlite", ".sqlite3":
		source, ok, err := datasource.SourceFromFile(path)
		if err != nil {
			return Source{}, err
		}
		if !ok {
			return Source{}, fmt.Errorf("unsupported SQLite source path: %s", path)
		}
		reader, err := datasource.NewSQLiteReader(source)
		if err != nil {
			return Source{}, err
		}
		defer reader.Close()
		loaded.Issues, err = reader.LoadIssueAuthority()
		if err != nil {
			return Source{}, err
		}
		report := reader.LoadReport()
		complete = report.Errors == 0 && report.ReadErrors == 0 && !report.Stale && len(report.AuthorityWarnings) == 0
	default:
		fullOpts := opts
		fullOpts.IssueFilter = nil
		var stats loader.ParseStats
		if fullOpts.Stats == nil {
			fullOpts.Stats = &stats
		}
		loaded, err = loader.LoadIssuesFromFileWithOptionsPooled(path, fullOpts)
		complete = fullOpts.Stats.Errors == 0
	}
	if err != nil {
		return Source{}, err
	}
	if degraded {
		complete = false
	}
	loader.AttachIssueOrigins(loaded.Issues, path, complete)
	if !isSQLitePath(path) && len(loaded.Issues) > 0 {
		if orig := loaded.Issues[0].Origin; orig != nil && orig.Tracker == "br" && orig.Database != "" {
			sibSrc := datasource.DataSource{Type: datasource.SourceTypeSQLite, Path: orig.Database}
			if datasource.ValidateSource(&sibSrc) != nil {
				complete = false
				for i := range loaded.Issues {
					if loaded.Issues[i].Origin != nil {
						loaded.Issues[i].Origin.Database = ""
						loaded.Issues[i].Origin.ReadOnlyReason = "sibling SQLite database is corrupt"
					}
				}
			}
		}
	}
	result := Source{
		Authority:         model.NewReadinessIndex(loaded.Issues),
		AuthorityHash:     analysis.ComputeDataHash(loaded.Issues),
		AuthorityComplete: complete,
	}
	loaded.Issues = IssuesWithoutTombstones(loaded.Issues)
	if opts.IssueFilter != nil {
		visible := loaded.Issues[:0]
		for i := range loaded.Issues {
			if opts.IssueFilter(&loaded.Issues[i]) {
				visible = append(visible, loaded.Issues[i])
			}
		}
		clear(loaded.Issues[len(visible):])
		loaded.Issues = visible
	}
	result.PooledIssues = loaded
	return result, nil
}

// LoadSource reads path and carries ctx's cancellation through every SQL
// statement. JSONL still uses its existing parser; cancellation there is checked
// before and after parsing, not claimed to interrupt filesystem reads or the
// parser. The same authority and filtering contract as LoadSourceSync applies.
func LoadSource(ctx context.Context, path string, opts loader.ParseOptions, degraded bool) (Source, error) {
	if err := ctx.Err(); err != nil {
		return Source{}, err
	}
	if !isSQLitePath(path) {
		loaded, err := LoadSourceSync(path, opts, degraded)
		if ctxErr := ctx.Err(); ctxErr != nil {
			loader.ReturnIssuePtrsToPool(loaded.PoolRefs)
			return Source{}, ctxErr
		}
		return loaded, err
	}
	reader, err := openSQLiteContext(ctx, path)
	if err != nil {
		return Source{}, err
	}
	defer reader.Close()
	issues, err := reader.LoadIssueAuthorityContext(ctx)
	if err != nil {
		return Source{}, err
	}
	report := reader.LoadReport()
	complete := report.Errors == 0 && report.ReadErrors == 0 && !report.Stale && len(report.AuthorityWarnings) == 0 && !degraded
	loader.AttachIssueOrigins(issues, path, complete)
	// Build authority before hiding satisfied tombstones or recipe-filtered
	// blockers. The read transaction is already closed before graph work.
	result := Source{
		Authority:         model.NewReadinessIndex(issues),
		AuthorityHash:     analysis.ComputeDataHash(issues),
		AuthorityComplete: complete,
	}
	visible := make([]model.Issue, 0, len(issues))
	for i := range issues {
		if err := ctx.Err(); err != nil {
			return Source{}, err
		}
		if !issues[i].Status.IsTombstone() && (opts.IssueFilter == nil || opts.IssueFilter(&issues[i])) {
			visible = append(visible, issues[i])
		}
	}
	if err := ctx.Err(); err != nil {
		return Source{}, err
	}
	result.Issues = visible
	return result, nil
}

// CountSourceSync returns the number of issues in path without a context: rows
// for SQLite, lines for JSONL.
func CountSourceSync(path string) (int, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".db", ".sqlite", ".sqlite3":
		source, ok, err := datasource.SourceFromFile(path)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, fmt.Errorf("unsupported SQLite source path: %s", path)
		}
		reader, err := datasource.NewReader(source)
		if err != nil {
			return 0, err
		}
		defer reader.Close()
		return reader.CountIssues()
	default:
		return CountJSONLLines(path)
	}
}

// CountSource is CountSourceSync with ctx cancellation.
func CountSource(ctx context.Context, path string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if !isSQLitePath(path) {
		count, err := CountSourceSync(path)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, ctxErr
		}
		return count, err
	}
	reader, err := openSQLiteContext(ctx, path)
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	return reader.CountIssuesContext(ctx)
}

// CountJSONLLines counts the lines of a file without parsing it. A final line
// without a trailing newline counts.
func CountJSONLLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	const bufSize = 32 * 1024
	buf := make([]byte, bufSize)
	lines := 0
	sawAny := false
	lastByte := byte(0)

	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			sawAny = true
			lastByte = buf[n-1]
			for _, b := range buf[:n] {
				if b == '\n' {
					lines++
				}
			}
		}
		if readErr == nil {
			continue
		}
		if readErr == io.EOF {
			break
		}
		return 0, readErr
	}

	if sawAny && lastByte != '\n' {
		lines++
	}
	return lines, nil
}

// IncludesClosedStatuses reports whether the recipe explicitly selects closed or
// tombstone issues, which disables the huge tier's open-only load.
func IncludesClosedStatuses(r *recipe.Recipe) bool {
	if r == nil {
		return false
	}
	for _, s := range r.Filters.Status {
		switch strings.TrimSpace(strings.ToLower(s)) {
		case string(model.StatusClosed), string(model.StatusTombstone):
			return true
		}
	}
	return false
}

// LargeDatasetWarning returns the short user-facing warning for large and huge
// tiers, or "" for the others.
func LargeDatasetWarning(t Tier, sourceHint, loaded int, openOnly bool) string {
	switch t {
	case TierLarge:
		n := loaded
		if sourceHint > 0 {
			n = sourceHint
		}
		return fmt.Sprintf("⚠ large %s issues", compactCount(n))
	case TierHuge:
		if openOnly && sourceHint > 0 {
			return fmt.Sprintf("⚠ huge open-only %s/%s", compactCount(loaded), compactCount(sourceHint))
		}
		n := loaded
		if sourceHint > 0 {
			n = sourceHint
		}
		return fmt.Sprintf("⚠ huge %s issues", compactCount(n))
	default:
		return ""
	}
}

func compactCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%dm", n/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dk", n/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// LoadOptions configures Load.
type LoadOptions struct {
	// Recipe derives Snapshot.ViewIssues; nil means no recipe.
	Recipe *recipe.Recipe
	// ProjectDir is the project root for .bv/ config; empty derives it from the
	// beads path.
	ProjectDir string
	// Degraded marks the source as incomplete authority.
	Degraded bool
	// AnalysisContext bounds the background Phase 2 analysis. Bind it to the
	// owner's lifetime (server or store), not to a request. nil = never cancelled.
	AnalysisContext context.Context
}

// Load reads beadsPath and builds a Phase 1 Snapshot the way the TUI's
// background worker does: it counts the source to pick a dataset tier, loads
// open-only issues for the huge tier (unless the recipe includes closed ones),
// hashes the data, builds with the tier's Config and feedback weights, and fills
// the load metadata.
//
// Phase 2 analysis starts in the background and is not tied to ctx, so a request
// context that ends after Load returns does not cancel it. Set
// opts.AnalysisContext to bound it to the owner's lifetime instead; use
// CompletePhase2 to wait for it. Pooled parser structs are not returned to the pool, because the
// snapshot outlives the call; they are left to the garbage collector.
func Load(ctx context.Context, beadsPath string, opts LoadOptions) (*Snapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// A failed count only disables tiering; the load below reports real errors.
	sourceCount := 0
	tier := TierUnknown
	if n, err := CountSource(ctx, beadsPath); err == nil {
		sourceCount = n
		tier = TierForIssueCount(n)
	} else if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	loadOpenOnly := tier == TierHuge && !IncludesClosedStatuses(opts.Recipe)

	loadWarningCount := 0
	parseOpts := loader.ParseOptions{
		WarningHandler: func(string) { loadWarningCount++ },
		BufferSize:     loader.MaxLineSizeFromEnv(),
	}
	if loadOpenOnly {
		parseOpts.IssueFilter = func(i *model.Issue) bool {
			return i.Status != model.StatusClosed && i.Status != model.StatusTombstone
		}
	}
	src, err := LoadSource(ctx, beadsPath, parseOpts, opts.Degraded)
	if err != nil {
		return nil, err
	}
	authorityComplete := src.AuthorityComplete && loadWarningCount == 0 && !opts.Degraded
	hash := analysis.ComputeDataHash(src.Issues)

	b := NewBuilder(src.Issues, src.Authority).
		WithBeadsPath(beadsPath).
		WithProjectDir(opts.ProjectDir).
		WithRecipe(opts.Recipe).
		WithWeights(FeedbackWeightsForBeadsPath(beadsPath)).
		WithConfig(ConfigForTier(tier))
	if opts.AnalysisContext != nil {
		b.WithContext(opts.AnalysisContext)
	}
	s := b.Build()

	s.DataHash = hash
	s.AuthorityHash = src.AuthorityHash
	s.AuthorityComplete = authorityComplete
	s.LoadWarningCount = loadWarningCount
	s.Tier = tier
	s.SourceIssueCountHint = sourceCount
	s.LoadedOpenOnly = loadOpenOnly
	if loadOpenOnly && sourceCount > len(s.Issues) {
		s.TruncatedCount = sourceCount - len(s.Issues)
	}
	s.LargeDatasetWarning = LargeDatasetWarning(tier, sourceCount, len(s.Issues), loadOpenOnly)
	return s, nil
}

// CompletePhase2 waits for s's Phase 2 analysis, then returns a new Snapshot
// with Phase 2 metrics, insights and triage populated. It uses a fresh analyzer
// that carries over s's readiness scope and scoring state, so the published
// snapshot's analyzer is never mutated. s itself is unchanged.
func (s *Snapshot) CompletePhase2(ctx context.Context) (*Snapshot, error) {
	if s == nil {
		return nil, nil
	}
	if s.Analysis == nil || s.Analyzer == nil {
		return nil, fmt.Errorf("snapshot has no analysis to complete")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	stats := s.Analysis
	if err := stats.WaitForPhase2Context(ctx); err != nil {
		return nil, err
	}
	issues := CloneIssues(s.Issues)
	analyzer := analysis.NewAnalyzer(issues)
	analyzer.SetReadinessScope(s.Analyzer.Readiness(), nil)
	analyzer.RestoreScoring(s.Analyzer.CaptureScoring())
	insights := stats.GenerateInsights(stats.NodeCount)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.WithPhase2(stats, insights, issues, analyzer), nil
}
