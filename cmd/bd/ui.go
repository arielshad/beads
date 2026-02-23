package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/ui"
)

type uiViewState int

const (
	uiViewList uiViewState = iota
	uiViewDetail
	uiViewHelp
)

type uiModel struct {
	ctx   context.Context
	store *dolt.DoltStore

	// Issue data
	allIssues []*types.Issue
	filtered  []*types.Issue
	labelsMap map[string][]string

	// List view state
	cursor int
	offset int

	// Detail view state
	detailIssue  *types.Issue
	detailScroll int
	detailLines  []string

	// View state
	view uiViewState

	// Terminal dimensions
	width  int
	height int

	// Search
	searching   bool
	searchQuery string

	// Filters
	statusFilter  int // index into statusFilters
	statusFilters []string

	// Error state
	err error
}

var statusFilterLabels = []string{
	"all open", "open", "in_progress", "blocked", "deferred", "closed", "all",
}

func initialModel(ctx context.Context, s *dolt.DoltStore) uiModel {
	return uiModel{
		ctx:           ctx,
		store:         s,
		statusFilter:  0,
		statusFilters: statusFilterLabels,
		view:          uiViewList,
	}
}

// --- Messages ---

type issuesLoadedMsg struct {
	issues    []*types.Issue
	labelsMap map[string][]string
}

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

// --- Commands ---

func loadIssues(ctx context.Context, s *dolt.DoltStore, statusFilter string) tea.Cmd {
	return func() tea.Msg {
		filter := types.IssueFilter{}

		switch statusFilter {
		case "all open":
			filter.ExcludeStatus = []types.Status{types.StatusClosed}
		case "all":
			// no filter
		default:
			st := types.Status(statusFilter)
			filter.Status = &st
		}

		isTemplate := false
		filter.IsTemplate = &isTemplate
		filter.ExcludeTypes = append(filter.ExcludeTypes, "gate")

		issues, err := s.SearchIssues(ctx, "", filter)
		if err != nil {
			return errMsg{err}
		}

		issueIDs := make([]string, len(issues))
		for i, issue := range issues {
			issueIDs[i] = issue.ID
		}
		labelsMap, _ := s.GetLabelsForIssues(ctx, issueIDs)

		return issuesLoadedMsg{issues: issues, labelsMap: labelsMap}
	}
}

// --- tea.Model implementation ---

func (m uiModel) Init() tea.Cmd {
	return loadIssues(m.ctx, m.store, m.statusFilters[m.statusFilter])
}

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.view == uiViewDetail && m.detailIssue != nil {
			m.detailLines = m.renderDetailLines(m.detailIssue)
		}
		return m, nil

	case issuesLoadedMsg:
		m.allIssues = msg.issues
		m.labelsMap = msg.labelsMap
		m.applySearch()
		m.clampCursor()
		if m.view == uiViewDetail && m.detailIssue != nil {
			m.detailLines = m.renderDetailLines(m.detailIssue)
		}
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m uiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	}

	if m.searching {
		return m.handleSearchKey(msg)
	}

	switch m.view {
	case uiViewList:
		return m.handleListKey(msg)
	case uiViewDetail:
		return m.handleDetailKey(msg)
	case uiViewHelp:
		return m.handleHelpKey(msg)
	}

	return m, nil
}

func (m uiModel) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.searching = false
		return m, nil
	case "esc":
		m.searching = false
		m.searchQuery = ""
		m.applySearch()
		m.clampCursor()
		return m, nil
	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.applySearch()
			m.clampCursor()
		}
		return m, nil
	default:
		if len(msg.String()) == 1 {
			m.searchQuery += msg.String()
			m.applySearch()
			m.clampCursor()
		}
		return m, nil
	}
}

func (m uiModel) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "?":
		m.view = uiViewHelp
		return m, nil
	case "j", "down":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.ensureVisible()
		}
		return m, nil
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.ensureVisible()
		}
		return m, nil
	case "g", "home":
		m.cursor = 0
		m.offset = 0
		return m, nil
	case "G", "end":
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
			m.ensureVisible()
		}
		return m, nil
	case "enter":
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			m.detailIssue = m.filtered[m.cursor]
			m.detailScroll = 0
			m.detailLines = m.renderDetailLines(m.detailIssue)
			m.view = uiViewDetail
		}
		return m, nil
	case "/":
		m.searching = true
		return m, nil
	case "f":
		m.statusFilter = (m.statusFilter + 1) % len(m.statusFilters)
		m.cursor = 0
		m.offset = 0
		return m, loadIssues(m.ctx, m.store, m.statusFilters[m.statusFilter])
	case "r":
		return m, loadIssues(m.ctx, m.store, m.statusFilters[m.statusFilter])
	case "pgdown":
		listH := m.listHeight()
		m.cursor += listH
		if m.cursor >= len(m.filtered) {
			m.cursor = len(m.filtered) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureVisible()
		return m, nil
	case "pgup":
		listH := m.listHeight()
		m.cursor -= listH
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureVisible()
		return m, nil
	}
	return m, nil
}

func (m uiModel) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "backspace":
		m.view = uiViewList
		m.detailIssue = nil
		m.detailScroll = 0
		m.detailLines = nil
		return m, nil
	case "j", "down":
		maxScroll := len(m.detailLines) - m.detailHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.detailScroll < maxScroll {
			m.detailScroll++
		}
		return m, nil
	case "k", "up":
		if m.detailScroll > 0 {
			m.detailScroll--
		}
		return m, nil
	case "pgdown":
		h := m.detailHeight()
		maxScroll := len(m.detailLines) - h
		if maxScroll < 0 {
			maxScroll = 0
		}
		m.detailScroll += h
		if m.detailScroll > maxScroll {
			m.detailScroll = maxScroll
		}
		return m, nil
	case "pgup":
		h := m.detailHeight()
		m.detailScroll -= h
		if m.detailScroll < 0 {
			m.detailScroll = 0
		}
		return m, nil
	case "g", "home":
		m.detailScroll = 0
		return m, nil
	case "G", "end":
		maxScroll := len(m.detailLines) - m.detailHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		m.detailScroll = maxScroll
		return m, nil
	case "?":
		m.view = uiViewHelp
		return m, nil
	}
	return m, nil
}

func (m uiModel) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "?", "backspace", "enter":
		if m.detailIssue != nil {
			m.view = uiViewDetail
		} else {
			m.view = uiViewList
		}
		return m, nil
	}
	return m, nil
}

// --- Helpers ---

func (m *uiModel) applySearch() {
	if m.searchQuery == "" {
		m.filtered = m.allIssues
		return
	}
	query := strings.ToLower(m.searchQuery)
	var result []*types.Issue
	for _, issue := range m.allIssues {
		if strings.Contains(strings.ToLower(issue.Title), query) ||
			strings.Contains(strings.ToLower(issue.ID), query) ||
			strings.Contains(strings.ToLower(string(issue.IssueType)), query) ||
			strings.Contains(strings.ToLower(issue.Assignee), query) {
			result = append(result, issue)
		}
	}
	m.filtered = result
}

func (m *uiModel) clampCursor() {
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureVisible()
}

func (m uiModel) listHeight() int {
	// header (1) + filter bar (1) + separator (1) + footer (1) + status bar (1) = 5
	h := m.height - 5
	if m.searching {
		h-- // search bar takes a line
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m uiModel) detailHeight() int {
	// header (1) + footer (1) = 2
	h := m.height - 2
	if h < 1 {
		h = 1
	}
	return h
}

func (m *uiModel) ensureVisible() {
	listH := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+listH {
		m.offset = m.cursor - listH + 1
	}
}

// --- View rendering ---

func (m uiModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	switch m.view {
	case uiViewList:
		return m.viewList()
	case uiViewDetail:
		return m.viewDetail()
	case uiViewHelp:
		return m.viewHelp()
	}
	return ""
}

// Styles for TUI
var (
	tuiHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#399ee6", Dark: "#59c2ff"})
	tuiSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.AdaptiveColor{Light: "#e8e8e8", Dark: "#333333"})
	tuiFooterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#828c99", Dark: "#6c7680"})
	tuiFilterActiveStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#399ee6", Dark: "#59c2ff"})
	tuiSearchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#f2ae49", Dark: "#ffb454"})
	tuiDividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#d4d4d4", Dark: "#444444"})
)

func (m uiModel) viewList() string {
	var b strings.Builder

	// Header
	title := tuiHeaderStyle.Render("bd ui")
	count := fmt.Sprintf(" %d issues", len(m.filtered))
	if len(m.filtered) != len(m.allIssues) {
		count = fmt.Sprintf(" %d/%d issues", len(m.filtered), len(m.allIssues))
	}
	b.WriteString(title + ui.RenderMuted(count))
	b.WriteString("\n")

	// Filter bar
	var filters []string
	for i, f := range m.statusFilters {
		if i == m.statusFilter {
			filters = append(filters, tuiFilterActiveStyle.Render("["+f+"]"))
		} else {
			filters = append(filters, ui.RenderMuted(f))
		}
	}
	b.WriteString(strings.Join(filters, ui.RenderMuted(" · ")))
	b.WriteString("\n")

	// Divider
	divWidth := m.width
	if divWidth > 80 {
		divWidth = 80
	}
	b.WriteString(tuiDividerStyle.Render(strings.Repeat("─", divWidth)))
	b.WriteString("\n")

	// Search bar
	if m.searching {
		b.WriteString(tuiSearchStyle.Render("/ ") + m.searchQuery + "█")
		b.WriteString("\n")
	}

	// Issue list
	listH := m.listHeight()

	if m.err != nil {
		b.WriteString(ui.RenderFail(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n")
	} else if len(m.filtered) == 0 {
		b.WriteString(ui.RenderMuted("  No issues found."))
		b.WriteString("\n")
	} else {
		end := m.offset + listH
		if end > len(m.filtered) {
			end = len(m.filtered)
		}
		for i := m.offset; i < end; i++ {
			issue := m.filtered[i]
			line := m.renderListItem(issue)

			if i == m.cursor {
				// Pad to width for selection highlight
				plainLen := lipgloss.Width(line)
				if plainLen < m.width {
					line += strings.Repeat(" ", m.width-plainLen)
				}
				line = tuiSelectedStyle.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}

		// Pad remaining lines
		rendered := end - m.offset
		for i := rendered; i < listH; i++ {
			b.WriteString("\n")
		}
	}

	// Footer / status bar
	if m.searching {
		b.WriteString(tuiFooterStyle.Render("Type to search · Enter to confirm · Esc to cancel"))
	} else {
		b.WriteString(tuiFooterStyle.Render("j/k navigate · Enter view · f filter · / search · r refresh · ? help · q quit"))
	}

	return b.String()
}

func (m uiModel) renderListItem(issue *types.Issue) string {
	statusIcon := ui.RenderStatusIcon(string(issue.Status))
	priorityTag := ui.RenderPriorityCompact(issue.Priority)

	typeBadge := ""
	switch issue.IssueType {
	case "epic":
		typeBadge = ui.TypeEpicStyle.Render("[epic]") + " "
	case "bug":
		typeBadge = ui.TypeBugStyle.Render("[bug]") + " "
	}

	assignee := ""
	if issue.Assignee != "" {
		assignee = " " + ui.RenderMuted("@"+issue.Assignee)
	}

	labels := m.labelsMap[issue.ID]
	labelStr := ""
	if len(labels) > 0 {
		labelStr = " " + ui.RenderMuted("["+strings.Join(labels, ",")+"]")
	}

	if issue.Status == types.StatusClosed {
		return fmt.Sprintf("%s %s %s %s%s%s%s",
			statusIcon,
			ui.RenderMuted(issue.ID),
			ui.RenderMuted(priorityTag),
			ui.RenderMuted(string(issue.IssueType)),
			ui.RenderMuted(" "+issue.Title),
			assignee,
			labelStr)
	}

	return fmt.Sprintf("%s %s %s %s%s%s%s",
		statusIcon,
		issue.ID,
		priorityTag,
		typeBadge,
		issue.Title,
		assignee,
		labelStr)
}

func (m uiModel) renderDetailLines(issue *types.Issue) []string {
	var lines []string
	w := m.width
	if w > 100 {
		w = 100
	}

	// Header
	lines = append(lines, formatIssueHeader(issue))
	lines = append(lines, formatIssueMetadata(issue))
	lines = append(lines, "")

	// Description
	if issue.Description != "" {
		lines = append(lines, ui.RenderBold("DESCRIPTION"))
		for _, l := range strings.Split(issue.Description, "\n") {
			lines = append(lines, "  "+l)
		}
		lines = append(lines, "")
	}

	// Design
	if issue.Design != "" {
		lines = append(lines, ui.RenderBold("DESIGN"))
		for _, l := range strings.Split(issue.Design, "\n") {
			lines = append(lines, "  "+l)
		}
		lines = append(lines, "")
	}

	// Notes
	if issue.Notes != "" {
		lines = append(lines, ui.RenderBold("NOTES"))
		for _, l := range strings.Split(issue.Notes, "\n") {
			lines = append(lines, "  "+l)
		}
		lines = append(lines, "")
	}

	// Acceptance Criteria
	if issue.AcceptanceCriteria != "" {
		lines = append(lines, ui.RenderBold("ACCEPTANCE CRITERIA"))
		for _, l := range strings.Split(issue.AcceptanceCriteria, "\n") {
			lines = append(lines, "  "+l)
		}
		lines = append(lines, "")
	}

	// Labels
	labels := m.labelsMap[issue.ID]
	if len(labels) > 0 {
		lines = append(lines, ui.RenderBold("LABELS")+"  "+strings.Join(labels, ", "))
		lines = append(lines, "")
	}

	// Metadata
	if metaStr := formatIssueCustomMetadata(issue); metaStr != "" {
		for _, l := range strings.Split(metaStr, "\n") {
			lines = append(lines, l)
		}
		lines = append(lines, "")
	}

	// Dependencies (load from store)
	depsWithMeta, _ := m.store.GetDependenciesWithMetadata(m.ctx, issue.ID)
	if len(depsWithMeta) > 0 {
		var blocks, parent []*types.IssueWithDependencyMetadata
		for _, dep := range depsWithMeta {
			switch dep.DependencyType {
			case types.DepParentChild:
				parent = append(parent, dep)
			default:
				blocks = append(blocks, dep)
			}
		}
		if len(parent) > 0 {
			lines = append(lines, ui.RenderBold("PARENT"))
			for _, dep := range parent {
				lines = append(lines, formatDependencyLine("↑", dep))
			}
			lines = append(lines, "")
		}
		if len(blocks) > 0 {
			lines = append(lines, ui.RenderBold("DEPENDS ON"))
			for _, dep := range blocks {
				lines = append(lines, formatDependencyLine("→", dep))
			}
			lines = append(lines, "")
		}
	}

	// Dependents
	dependentsWithMeta, _ := m.store.GetDependentsWithMetadata(m.ctx, issue.ID)
	if len(dependentsWithMeta) > 0 {
		var blocks, children []*types.IssueWithDependencyMetadata
		for _, dep := range dependentsWithMeta {
			switch dep.DependencyType {
			case types.DepParentChild:
				children = append(children, dep)
			default:
				blocks = append(blocks, dep)
			}
		}
		if len(children) > 0 {
			lines = append(lines, ui.RenderBold("CHILDREN"))
			for _, dep := range children {
				lines = append(lines, formatDependencyLine("↳", dep))
			}
			lines = append(lines, "")
		}
		if len(blocks) > 0 {
			lines = append(lines, ui.RenderBold("BLOCKS"))
			for _, dep := range blocks {
				lines = append(lines, formatDependencyLine("←", dep))
			}
			lines = append(lines, "")
		}
	}

	// Comments
	comments, _ := m.store.GetIssueComments(m.ctx, issue.ID)
	if len(comments) > 0 {
		lines = append(lines, ui.RenderBold("COMMENTS"))
		for _, comment := range comments {
			lines = append(lines, fmt.Sprintf("  %s %s",
				ui.RenderMuted(comment.CreatedAt.UTC().Format("2006-01-02 15:04")),
				comment.Author))
			for _, l := range strings.Split(strings.TrimRight(comment.Text, "\n"), "\n") {
				lines = append(lines, "    "+l)
			}
			lines = append(lines, "")
		}
	}

	// Word-wrap long lines
	var wrapped []string
	for _, line := range lines {
		if lipgloss.Width(line) > w && w > 10 {
			wrapped = append(wrapped, wrapLine(line, w))
		} else {
			wrapped = append(wrapped, line)
		}
	}

	return wrapped
}

func wrapLine(line string, maxWidth int) string {
	if lipgloss.Width(line) <= maxWidth {
		return line
	}
	// Simple wrap: split on spaces, reassemble
	words := strings.Fields(line)
	if len(words) == 0 {
		return line
	}

	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		test := current + " " + word
		if lipgloss.Width(test) > maxWidth {
			lines = append(lines, current)
			current = "  " + word
		} else {
			current = test
		}
	}
	lines = append(lines, current)
	return strings.Join(lines, "\n")
}

func (m uiModel) viewDetail() string {
	if m.detailIssue == nil {
		return "No issue selected"
	}

	var b strings.Builder

	h := m.detailHeight()
	end := m.detailScroll + h
	if end > len(m.detailLines) {
		end = len(m.detailLines)
	}

	start := m.detailScroll
	if start > len(m.detailLines) {
		start = len(m.detailLines)
	}

	for i := start; i < end; i++ {
		b.WriteString(m.detailLines[i])
		b.WriteString("\n")
	}

	// Pad remaining
	rendered := end - start
	for i := rendered; i < h; i++ {
		b.WriteString("\n")
	}

	// Footer
	scrollInfo := ""
	if len(m.detailLines) > h {
		pct := 0
		if len(m.detailLines)-h > 0 {
			pct = m.detailScroll * 100 / (len(m.detailLines) - h)
		}
		scrollInfo = fmt.Sprintf(" · %d%%", pct)
	}
	b.WriteString(tuiFooterStyle.Render(
		"j/k scroll · PgUp/PgDn page · g/G top/bottom · Esc back · ? help · q quit" + scrollInfo))

	return b.String()
}

func (m uiModel) viewHelp() string {
	var b strings.Builder

	b.WriteString(tuiHeaderStyle.Render("Keyboard Shortcuts"))
	b.WriteString("\n\n")

	helpSections := []struct {
		title string
		keys  []struct{ key, desc string }
	}{
		{
			title: "Navigation",
			keys: []struct{ key, desc string }{
				{"j / ↓", "Move cursor down"},
				{"k / ↑", "Move cursor up"},
				{"g / Home", "Go to first item"},
				{"G / End", "Go to last item"},
				{"PgDn", "Page down"},
				{"PgUp", "Page up"},
			},
		},
		{
			title: "Actions",
			keys: []struct{ key, desc string }{
				{"Enter", "View issue details"},
				{"Esc / Backspace", "Go back"},
				{"/ ", "Search issues"},
				{"f", "Cycle status filter"},
				{"r", "Refresh issue list"},
			},
		},
		{
			title: "General",
			keys: []struct{ key, desc string }{
				{"?", "Toggle this help"},
				{"q", "Quit"},
				{"Ctrl+C", "Force quit"},
			},
		},
	}

	for _, section := range helpSections {
		b.WriteString(ui.RenderBold(section.title))
		b.WriteString("\n")
		for _, k := range section.keys {
			b.WriteString(fmt.Sprintf("  %-20s %s\n", tuiFilterActiveStyle.Render(k.key), k.desc))
		}
		b.WriteString("\n")
	}

	b.WriteString(tuiFooterStyle.Render("Press ? or Esc to close help"))

	return b.String()
}

// --- Command ---

var uiCmd = &cobra.Command{
	Use:     "ui",
	GroupID: "views",
	Short:   "Interactive terminal UI for browsing issues",
	Long: `Launch an interactive terminal interface for browsing and viewing issues.

The UI provides:
  - Scrollable issue list with status/priority indicators
  - Issue detail view with full content rendering
  - Real-time search and status filtering
  - Keyboard-driven navigation (vim-style keys supported)

Keyboard shortcuts:
  j/k or ↑/↓    Navigate issues
  Enter          View issue details
  Esc            Go back / close
  /              Search issues
  f              Cycle status filter
  r              Refresh
  ?              Show help
  q              Quit`,
	Run: func(cmd *cobra.Command, _ []string) {
		_ = cmd
		ctx := rootCtx

		m := initialModel(ctx, store)
		p := tea.NewProgram(m, tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running UI: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(uiCmd)
}
