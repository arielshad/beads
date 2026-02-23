// ui.go - UI discovery and support for beads
// Helps users find and launch community-built UIs (TUIs, web, editor extensions).

package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/ui"
)

// uiEntry represents a community UI with install/run instructions
type uiEntry struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	RunCommand  string `json:"run_command,omitempty"`
	URL         string `json:"url,omitempty"`
	Language    string `json:"language,omitempty"`
}

// Curated list of community UIs - see docs/COMMUNITY_TOOLS.md for full list
var curatedUIs = []uiEntry{
	// Terminal UIs
	{Name: "beads_viewer", Category: "terminal", Description: "Elegant, keyboard-driven TUI with tree navigation (Go)", URL: "https://github.com/Dicklesworthstone/beads_viewer"},
	{Name: "bdui", Category: "terminal", Description: "Real-time TUI with tree view and dependency graph (Node.js)", URL: "https://github.com/assimelha/bdui"},
	{Name: "perles", Category: "terminal", Description: "TUI search, dependency and kanban viewer with BQL (Go)", URL: "https://github.com/zjrosen/perles"},
	{Name: "lazybeads", Category: "terminal", Description: "Lightweight Bubble Tea TUI (Go)", URL: "https://github.com/codegangsta/lazybeads"},
	{Name: "bsv", Category: "terminal", Description: "Two-panel TUI with tree by epic/task/sub-task (Rust)", URL: "https://github.com/bglenden/bsv"},
	{Name: "abacus", Category: "terminal", Description: "Terminal UI for visualizing and navigating issues", URL: "https://github.com/ChrisEdwards/abacus"},
	// Web UIs
	{Name: "beads-ui", Category: "web", Description: "Local web interface with live updates and kanban", RunCommand: "npx beads-ui start", URL: "https://github.com/mantoni/beads-ui"},
	{Name: "beads-dashboard", Category: "web", Description: "Metrics dashboard (lead time, throughput)", URL: "https://github.com/rhydlewis/beads-dashboard"},
	{Name: "beadsmap", Category: "web", Description: "Interactive roadmap with Gantt, list, table views (Svelte)", URL: "https://github.com/dariye/beadsmap"},
	// Editor extensions
	{Name: "vscode-beads", Category: "editor", Description: "VS Code extension with issues panel", URL: "https://marketplace.visualstudio.com/items?itemName=planet57.vscode-beads"},
	{Name: "nvim-beads", Category: "editor", Description: "Neovim plugin for managing beads (Lua)", URL: "https://github.com/joeblubaugh/nvim-beads"},
	{Name: "beads.el", Category: "editor", Description: "Emacs UI to browse, edit, and manage beads", URL: "https://codeberg.org/ctietze/beads.el"},
	// Native apps
	{Name: "Beadbox", Category: "native", Description: "macOS dashboard with real-time sync", RunCommand: "brew tap beadbox/cask && brew install --cask beadbox", URL: "https://github.com/beadbox/beadbox"},
	{Name: "Beadster", Category: "native", Description: "macOS app for browsing issues", URL: "https://github.com/podviaznikov/beadster"},
}

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Discover and launch beads UIs",
	Long: `List community-built UIs for beads: terminal UIs (TUIs), web interfaces,
editor extensions, and native apps. Use --json for machine-readable output.

See docs/COMMUNITY_TOOLS.md and GitHub Discussions #276 for full listings.`,
	GroupID: "advanced",
	RunE:    runUIList,
}

var uiListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available UIs",
	Long:  "List community-built terminal, web, editor, and native UIs for beads.",
	RunE:  runUIList,
}

func init() {
	rootCmd.AddCommand(uiCmd)
	uiCmd.AddCommand(uiListCmd)
}

func runUIList(_ *cobra.Command, _ []string) error {
	if jsonOutput {
		return outputUIListJSON(curatedUIs)
	}
	return outputUIListHuman(curatedUIs)
}

func outputUIListJSON(entries []uiEntry) error {
	result := map[string]interface{}{
		"uis":        entries,
		"docs_url":   "https://github.com/steveyegge/beads/blob/main/docs/COMMUNITY_TOOLS.md",
		"discussion": "https://github.com/steveyegge/beads/discussions/276",
	}
	outputJSON(result)
	return nil
}

func outputUIListHuman(entries []uiEntry) error {
	fmt.Println(ui.RenderCategory("Available Beads UIs"))
	fmt.Println(ui.RenderMuted("Community-built interfaces. See docs/COMMUNITY_TOOLS.md for full list."))
	fmt.Println()

	// Group by category
	cats := map[string][]uiEntry{
		"terminal": nil,
		"web":      nil,
		"editor":   nil,
		"native":   nil,
	}
	for _, e := range entries {
		cats[e.Category] = append(cats[e.Category], e)
	}

	order := []string{"terminal", "web", "editor", "native"}
	titles := map[string]string{
		"terminal": "Terminal UIs",
		"web":      "Web UIs",
		"editor":   "Editor Extensions",
		"native":   "Native Apps",
	}

	for _, cat := range order {
		list := cats[cat]
		if len(list) == 0 {
			continue
		}
		fmt.Println(ui.RenderAccent(titles[cat] + ":"))
		for _, e := range list {
			name := ui.RenderBold(e.Name)
			desc := e.Description
			if e.RunCommand != "" {
				desc += " — " + ui.RenderCommand(e.RunCommand)
			}
			fmt.Printf("  %s - %s\n", name, desc)
			if e.URL != "" {
				fmt.Printf("    %s\n", ui.RenderMuted(e.URL))
			}
		}
		fmt.Println()
	}

	fmt.Println(ui.RenderSeparator())
	fmt.Println(ui.RenderMuted("Full list: docs/COMMUNITY_TOOLS.md"))
	fmt.Println(ui.RenderMuted("Discussion: https://github.com/steveyegge/beads/discussions/276"))
	return nil
}
