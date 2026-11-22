package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

func agentdeskTestIssues(now time.Time) []model.Issue {
	started := now.Add(-5 * time.Hour)
	return []model.Issue{
		{ID: "ep-1", Title: "Epic one", Status: model.StatusOpen, IssueType: model.TypeEpic, UpdatedAt: now},
		{
			ID: "ep-1.1", Title: "Pick a store", Status: model.StatusOpen, IssueType: model.TypeTask,
			Labels: []string{"needs-decision"}, Priority: 1, UpdatedAt: now,
			Description:  "Waiting on: your call between stores.\n\nOptions:\na) SQLite (recommended)\nb) Postgres",
			Dependencies: []*model.Dependency{{IssueID: "ep-1.1", DependsOnID: "ep-1", Type: model.DepParentChild}},
		},
		{
			ID: "ep-1.2", Title: "Slice done", Status: model.StatusClosed, IssueType: model.TypeTask, UpdatedAt: now,
			Metadata:     []byte(`{"slice_outcome":"committed","review_rounds":1}`),
			Dependencies: []*model.Dependency{{IssueID: "ep-1.2", DependsOnID: "ep-1", Type: model.DepParentChild}},
		},
		{
			ID: "ep-1.3", Title: "Working", Status: model.StatusInProgress, IssueType: model.TypeTask, UpdatedAt: started,
			StartedAt: &started, Assignee: "agent",
			Metadata:     []byte(`{"stage":"coding","claim.session":"s1"}`),
			Dependencies: []*model.Dependency{{IssueID: "ep-1.3", DependsOnID: "ep-1", Type: model.DepParentChild}},
		},
	}
}

func runAgentdeskHandler(t *testing.T, h func(RobotContext) error, issues []model.Issue) map[string]interface{} {
	t.Helper()
	var buf bytes.Buffer
	ctx := RobotContext{Issues: issues, Encoder: json.NewEncoder(&buf)}
	if err := h(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	return out
}

// requireOutputMatchesSchema checks that top-level output keys are declared by
// the schema and that every required property is present.
func requireOutputMatchesSchema(t *testing.T, command string, out map[string]interface{}) {
	t.Helper()
	schemas := generateRobotSchemas()
	props := requireRobotSchemaProperties(t, schemas, command)
	sharedEnvelope := map[string]bool{"source_path": true, "source_kind": true, "as_of": true, "as_of_commit": true, "scope": true, "load_stats": true, "source_authority": true, "authority_hash": true, "scope_hash": true}
	for key := range out {
		if props[key] == nil && !sharedEnvelope[key] {
			t.Errorf("%s output key %q not in schema", command, key)
		}
	}
	required, _ := schemas.Commands[command]["required"].([]string)
	for _, key := range required {
		if _, ok := out[key]; !ok {
			t.Errorf("%s output missing required key %q", command, key)
		}
	}
}

func TestRobotInboxHandlerAndSchema(t *testing.T) {
	now := time.Now().UTC()
	out := runAgentdeskHandler(t, handleRobotInbox, agentdeskTestIssues(now))
	requireOutputMatchesSchema(t, "robot-inbox", out)
	if out["total"].(float64) != 1 {
		t.Fatalf("total = %v, want 1", out["total"])
	}
	groups := out["groups"].([]interface{})
	g := groups[0].(map[string]interface{})
	if g["epic"] != "ep-1" || g["epic_title"] != "Epic one" {
		t.Fatalf("unexpected group: %v", g)
	}
	item := g["items"].([]interface{})[0].(map[string]interface{})
	if item["id"] != "ep-1.1" {
		t.Fatalf("unexpected item: %v", item)
	}
	groupSchema := generateRobotSchemas().Commands["robot-inbox"]["properties"].(map[string]interface{})["groups"].(map[string]interface{})["items"]
	itemSchema := requireNestedSchemaProperties(t, groupSchema, "group")["items"]
	itemProps := requireNestedSchemaProperties(t, itemSchema.(map[string]interface{})["items"], "item")
	for key := range item {
		if itemProps[key] == nil {
			t.Errorf("inbox item key %q not in schema", key)
		}
	}

	empty := runAgentdeskHandler(t, handleRobotInbox, nil)
	if g, ok := empty["groups"].([]interface{}); !ok || len(g) != 0 {
		t.Fatalf("empty inbox groups = %v, want []", empty["groups"])
	}
}

func TestRobotPipelineHandlerAndSchema(t *testing.T) {
	now := time.Now().UTC()
	issues := agentdeskTestIssues(now)
	var buf bytes.Buffer
	stale := "1h"
	ctx := RobotContext{Issues: issues, Encoder: json.NewEncoder(&buf)}
	if err := handleRobotPipeline(ctx, phaseThreeRobotHandlerConfig{StaleAfter: &stale}); err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	requireOutputMatchesSchema(t, "robot-pipeline", out)
	if out["total"].(float64) != 1 || out["stale"].(float64) != 1 || out["stale_after"] != "1h" {
		t.Fatalf("unexpected pipeline summary: %v", out)
	}
	row := out["rows"].([]interface{})[0].(map[string]interface{})
	if row["stage"] != "coding" || row["session"] != "s1" || row["stale"] != true {
		t.Fatalf("unexpected row: %v", row)
	}
	rowProps := requireNestedSchemaProperties(t,
		generateRobotSchemas().Commands["robot-pipeline"]["properties"].(map[string]interface{})["rows"].(map[string]interface{})["items"], "row")
	for key := range row {
		if rowProps[key] == nil {
			t.Errorf("pipeline row key %q not in schema", key)
		}
	}

	bad := "soon"
	err := handleRobotPipeline(RobotContext{Issues: issues, Encoder: json.NewEncoder(&bytes.Buffer{})}, phaseThreeRobotHandlerConfig{StaleAfter: &bad})
	if err == nil {
		t.Fatal("expected error for invalid --stale-after")
	}
}

func TestRobotSliceMetricsHandlerAndSchema(t *testing.T) {
	out := runAgentdeskHandler(t, handleRobotSliceMetrics, agentdeskTestIssues(time.Now().UTC()))
	requireOutputMatchesSchema(t, "robot-slice-metrics", out)
	total := out["total"].(map[string]interface{})
	if total["slices"].(float64) != 1 || total["committed"].(float64) != 1 || total["review_rounds"].(float64) != 1 {
		t.Fatalf("unexpected total: %v", total)
	}
	metricProps := requireNestedSchemaProperties(t, generateRobotSchemas().Commands["robot-slice-metrics"]["properties"].(map[string]interface{})["total"], "metrics")
	for key := range total {
		if metricProps[key] == nil {
			t.Errorf("slice metrics key %q not in schema", key)
		}
	}
	epics := out["epics"].([]interface{})
	if len(epics) != 1 || epics[0].(map[string]interface{})["epic"] != "ep-1" {
		t.Fatalf("unexpected epics: %v", epics)
	}
}
