package main

import (
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/ui"
)

func TestRenderTodoListOutput_Empty(t *testing.T) {
	got := renderTodoListOutput(nil, false)
	if got != "No TODOs found\n" {
		t.Fatalf("unexpected empty output: %q", got)
	}
}

func TestRenderTodoListOutput_Plain(t *testing.T) {
	issues := []*types.Issue{
		{
			ID:       "bd-100",
			Title:    "Write integration test",
			Priority: 1,
			Status:   types.StatusOpen,
		},
		{
			ID:       "bd-101",
			Title:    "Polish docs",
			Priority: 3,
			Status:   types.StatusInProgress,
		},
	}

	got := renderTodoListOutput(issues, true)

	if !strings.Contains(got, "1. [P1] bd-100: Write integration test (open)") {
		t.Fatalf("plain output missing first issue: %q", got)
	}
	if !strings.Contains(got, "2. [P3] bd-101: Polish docs (in_progress)") {
		t.Fatalf("plain output missing second issue: %q", got)
	}
	if strings.Contains(got, ui.StatusIconOpen) {
		t.Fatalf("plain output should not include status icons: %q", got)
	}
	if !strings.Contains(got, "Total: 2 TODOs") {
		t.Fatalf("plain output missing summary: %q", got)
	}
}

func TestRenderTodoListOutput_Pretty(t *testing.T) {
	issues := []*types.Issue{
		{
			ID:       "bd-200",
			Title:    "Short title",
			Priority: 0,
			Status:   types.StatusOpen,
		},
		{
			ID:       "bd-201",
			Title:    "A very long title that should be truncated for pretty tabular display output",
			Priority: 2,
			Status:   types.StatusBlocked,
		},
	}

	got := renderTodoListOutput(issues, false)
	if !strings.Contains(got, ui.StatusIconOpen) {
		t.Fatalf("pretty output should include open status icon: %q", got)
	}
	if !strings.Contains(got, ui.StatusIconBlocked) {
		t.Fatalf("pretty output should include blocked status icon: %q", got)
	}
	if !strings.Contains(got, "bd-200") || !strings.Contains(got, "bd-201") {
		t.Fatalf("pretty output missing IDs: %q", got)
	}
	if !strings.Contains(got, todoTruncate(issues[1].Title, 40)) {
		t.Fatalf("pretty output should truncate long titles: %q", got)
	}
	if !strings.Contains(got, "Total: 2 TODOs") {
		t.Fatalf("pretty output missing summary: %q", got)
	}
}

func TestTodoTruncate(t *testing.T) {
	if got := todoTruncate("short", 10); got != "short" {
		t.Fatalf("unexpected short truncate result: %q", got)
	}
	if got := todoTruncate("1234567890", 10); got != "1234567890" {
		t.Fatalf("unexpected equal-length truncate result: %q", got)
	}
	if got := todoTruncate("12345678901", 10); got != "1234567..." {
		t.Fatalf("unexpected long truncate result: %q", got)
	}
}

func TestTodoSortIssues(t *testing.T) {
	issues := []*types.Issue{
		{ID: "bd-b", Priority: 2},
		{ID: "bd-a", Priority: 2},
		{ID: "bd-z", Priority: 0},
	}

	todoSortIssues(issues)

	got := []string{issues[0].ID, issues[1].ID, issues[2].ID}
	want := []string{"bd-z", "bd-a", "bd-b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected sort order at %d: got %v want %v", i, got, want)
		}
	}
}
