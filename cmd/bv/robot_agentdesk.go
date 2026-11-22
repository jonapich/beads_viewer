package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// agentdeskIssues returns the full issue set (including closed beads) so epic
// titles, parent chains and slice metrics are resolved against every bead, not
// just the display-filtered view.
func agentdeskIssues(ctx RobotContext) []model.Issue {
	if len(ctx.FullIssues) > 0 {
		return ctx.FullIssues
	}
	return ctx.Issues
}

// handleRobotInbox emits the beads awaiting the user, grouped by epic.
func handleRobotInbox(ctx RobotContext) error {
	inbox := agentdesk.BuildInbox(agentdeskIssues(ctx), nil)
	if inbox.Groups == nil {
		inbox.Groups = []agentdesk.InboxGroup{}
	}
	output := struct {
		RobotEnvelope
		agentdesk.Inbox
	}{
		RobotEnvelope: ctx.Envelope(),
		Inbox:         inbox,
	}
	if err := ctx.EncoderOrDefault().Encode(output); err != nil {
		return fmt.Errorf("encoding inbox: %w", err)
	}
	return nil
}

// handleRobotPipeline emits the in-progress beads with stage and staleness.
func handleRobotPipeline(ctx RobotContext, cfg phaseThreeRobotHandlerConfig) error {
	staleAfter := agentdesk.DefaultStaleAfter
	if cfg.StaleAfter != nil && strings.TrimSpace(*cfg.StaleAfter) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(*cfg.StaleAfter))
		if err != nil || d <= 0 {
			return fmt.Errorf("invalid --stale-after %q: want a positive duration such as 2h or 90m", *cfg.StaleAfter)
		}
		staleAfter = d
	}
	pipeline := agentdesk.BuildPipeline(agentdeskIssues(ctx), ctx.ReferenceNow(), staleAfter)
	if pipeline.Rows == nil {
		pipeline.Rows = []agentdesk.PipelineRow{}
	}
	if pipeline.ByStage == nil {
		pipeline.ByStage = map[string]int{}
	}
	// Pipeline carries its own generated_at, which would collide with the
	// envelope's and make encoding/json drop both, so its fields are listed here.
	output := struct {
		RobotEnvelope
		StaleAfter string                  `json:"stale_after"`
		Total      int                     `json:"total"`
		Stale      int                     `json:"stale"`
		ByStage    map[string]int          `json:"by_stage"`
		Rows       []agentdesk.PipelineRow `json:"rows"`
	}{
		RobotEnvelope: ctx.Envelope(),
		StaleAfter:    pipeline.StaleAfter,
		Total:         pipeline.Total,
		Stale:         pipeline.Stale,
		ByStage:       pipeline.ByStage,
		Rows:          pipeline.Rows,
	}
	if err := ctx.EncoderOrDefault().Encode(output); err != nil {
		return fmt.Errorf("encoding pipeline: %w", err)
	}
	return nil
}

// handleRobotSliceMetrics emits per-epic slice metrics.
func handleRobotSliceMetrics(ctx RobotContext) error {
	report := agentdesk.BuildSliceReport(agentdeskIssues(ctx))
	if report.Epics == nil {
		report.Epics = []agentdesk.SliceMetrics{}
	}
	output := struct {
		RobotEnvelope
		Total            agentdesk.SliceMetrics   `json:"total"`
		Epics            []agentdesk.SliceMetrics `json:"epics"`
		UnmatchedEscapes []agentdesk.EscapedBug   `json:"unmatched_escapes,omitempty"`
	}{
		RobotEnvelope:    ctx.Envelope(),
		Total:            report.Total,
		Epics:            report.Epics,
		UnmatchedEscapes: report.UnmatchedEscapes,
	}
	if err := ctx.EncoderOrDefault().Encode(output); err != nil {
		return fmt.Errorf("encoding slice metrics: %w", err)
	}
	return nil
}

func schemaStr() map[string]interface{}  { return map[string]interface{}{"type": "string"} }
func schemaInt() map[string]interface{}  { return map[string]interface{}{"type": "integer"} }
func schemaBool() map[string]interface{} { return map[string]interface{}{"type": "boolean"} }
func schemaTime() map[string]interface{} {
	return map[string]interface{}{"type": "string", "format": "date-time"}
}
func schemaArrayOf(items map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"type": "array", "items": items}
}

func agentdeskEnvelopeProps(props map[string]interface{}) map[string]interface{} {
	props["generated_at"] = schemaTime()
	props["data_hash"] = schemaStr()
	props["version"] = schemaStr()
	return props
}

func robotInboxSchema() map[string]interface{} {
	link := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"kind": schemaStr(), "url": schemaStr(), "note": schemaStr(),
			"at": schemaTime(), "by": schemaStr(), "source": schemaStr(),
		},
		"required": []string{"kind", "url", "source"},
	}
	option := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"key": schemaStr(), "text": schemaStr(), "recommended": schemaBool(),
		},
		"required": []string{"key", "text"},
	}
	item := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": schemaStr(), "title": schemaStr(), "label": schemaStr(),
			"awaiting_labels": schemaArrayOf(schemaStr()),
			"priority":        schemaInt(), "status": schemaStr(), "epic": schemaStr(),
			"ready_to_judge": schemaBool(), "waiting_on": schemaStr(),
			"links":      schemaArrayOf(link),
			"options":    schemaArrayOf(option),
			"updated_at": schemaTime(),
		},
		"required": []string{"id", "title", "label", "awaiting_labels", "priority", "status", "ready_to_judge", "updated_at"},
	}
	group := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"epic": schemaStr(), "epic_title": schemaStr(),
			"ready_to_judge": schemaInt(), "items": schemaArrayOf(item),
		},
		"required": []string{"epic", "ready_to_judge", "items"},
	}
	return map[string]interface{}{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"title":       "Robot Inbox Output",
		"description": "Beads awaiting the user (open + needs-decision/needs-design), grouped by epic",
		"type":        "object",
		"properties": agentdeskEnvelopeProps(map[string]interface{}{
			"total": schemaInt(), "ready_to_judge": schemaInt(), "groups": schemaArrayOf(group),
		}),
		"required": []string{"generated_at", "data_hash", "total", "ready_to_judge", "groups"},
	}
}

func robotPipelineSchema() map[string]interface{} {
	row := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": schemaStr(), "title": schemaStr(), "epic": schemaStr(), "stage": schemaStr(),
			"stage_since": schemaTime(), "stage_age": schemaStr(), "stage_age_seconds": schemaInt(),
			"holder": schemaStr(), "session": schemaStr(),
			"claimed_at": schemaTime(), "lease_expires_at": schemaTime(), "heartbeat_at": schemaTime(),
			"last_activity": schemaTime(), "stale": schemaBool(), "stale_reason": schemaStr(),
		},
		"required": []string{"id", "title", "stage_age_seconds", "last_activity", "stale"},
	}
	return map[string]interface{}{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"title":       "Robot Pipeline Output",
		"description": "In-progress beads with stage, session, stage age and stale flag",
		"type":        "object",
		"properties": agentdeskEnvelopeProps(map[string]interface{}{
			"stale_after": schemaStr(), "total": schemaInt(), "stale": schemaInt(),
			"by_stage": map[string]interface{}{"type": "object", "additionalProperties": schemaInt()},
			"rows":     schemaArrayOf(row),
		}),
		"required": []string{"generated_at", "data_hash", "stale_after", "total", "stale", "by_stage", "rows"},
	}
}

func robotSliceMetricsSchema() map[string]interface{} {
	metrics := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"epic": schemaStr(), "epic_title": schemaStr(), "slices": schemaInt(),
			"committed": schemaInt(), "stopped": schemaInt(), "review_rounds": schemaInt(),
			"first_pass_approvals": schemaInt(), "review_blocking": schemaInt(),
			"test_runs": schemaInt(), "test_red_runs": schemaInt(), "test_timeouts": schemaInt(),
			"test_wall_minutes": schemaInt(), "slice_wall_minutes": schemaInt(),
			"followup_beads": schemaInt(), "coder_tokens": schemaInt(), "reviewer_tokens": schemaInt(),
			"other_agent_tokens": schemaInt(), "slices_with_unknown_tokens": schemaInt(),
			"escaped_bugs": schemaInt(),
		},
		"required": []string{"epic", "slices", "committed", "stopped", "review_rounds", "escaped_bugs"},
	}
	escape := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"bug": schemaStr(), "cause": schemaStr()},
		"required":   []string{"bug", "cause"},
	}
	return map[string]interface{}{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"title":       "Robot Slice Metrics Output",
		"description": "Per-epic slice metrics summary with a grand total",
		"type":        "object",
		"properties": agentdeskEnvelopeProps(map[string]interface{}{
			"total": metrics, "epics": schemaArrayOf(metrics), "unmatched_escapes": schemaArrayOf(escape),
		}),
		"required": []string{"generated_at", "data_hash", "total", "epics"},
	}
}
