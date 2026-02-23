package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestUICommand(t *testing.T) {
	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	t.Run("human output lists UIs", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("Failed to create pipe: %v", err)
		}
		os.Stdout = w
		jsonOutput = false

		err = uiCmd.RunE(uiCmd, []string{})
		w.Close()
		if err != nil {
			t.Fatalf("ui command failed: %v", err)
		}

		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output := buf.String()

		if !strings.Contains(output, "beads_viewer") {
			t.Errorf("Expected output to list beads_viewer, got: %s", output)
		}
		if !strings.Contains(output, "beads-ui") {
			t.Errorf("Expected output to list beads-ui, got: %s", output)
		}
		if !strings.Contains(output, "COMMUNITY_TOOLS") {
			t.Errorf("Expected output to reference COMMUNITY_TOOLS, got: %s", output)
		}
	})

	t.Run("json output is valid", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("Failed to create pipe: %v", err)
		}
		os.Stdout = w
		jsonOutput = true

		err = uiCmd.RunE(uiCmd, []string{})
		w.Close()
		if err != nil {
			t.Fatalf("ui command failed: %v", err)
		}

		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output := buf.String()

		var result struct {
			UIs       []uiEntry `json:"uis"`
			DocsURL   string   `json:"docs_url"`
			Discussion string  `json:"discussion"`
		}
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, output)
		}
		if len(result.UIs) == 0 {
			t.Error("Expected at least one UI in JSON output")
		}
		if result.DocsURL == "" {
			t.Error("Expected docs_url in JSON output")
		}
	})
}
