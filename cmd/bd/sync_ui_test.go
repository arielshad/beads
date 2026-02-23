package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/tracker"
)

func TestPrintSyncSummary_DryRun(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	printSyncSummary(&out, "Linear", &tracker.SyncResult{}, true)

	got := out.String()
	if !strings.Contains(got, "Dry run complete") {
		t.Fatalf("expected dry-run summary, got %q", got)
	}
	if strings.Contains(got, "sync complete") {
		t.Fatalf("did not expect provider completion line in dry-run summary, got %q", got)
	}
}

func TestPrintSyncSummary_WithStatsAndWarnings(t *testing.T) {
	t.Parallel()

	result := &tracker.SyncResult{
		Stats: tracker.SyncStats{
			Pulled:    3,
			Pushed:    2,
			Created:   1,
			Updated:   2,
			Conflicts: 1,
		},
		Warnings: []string{"rate limit approaching"},
	}

	var out bytes.Buffer
	printSyncSummary(&out, "Jira", result, false)
	got := out.String()

	checks := []string{
		"Pulled 3 issues (1 created, 2 updated)",
		"Pushed 2 issues",
		"Resolved 1 conflicts",
		"Jira sync complete",
		"Warnings:",
		"rate limit approaching",
	}

	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Fatalf("expected output to contain %q, got %q", check, got)
		}
	}
}
