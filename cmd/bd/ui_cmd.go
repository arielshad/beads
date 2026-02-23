package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/ui"
)

// --- Styles ---

var (
	uiTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#399ee6", Dark: "#59c2ff"}).
			Padding(0, 1)

	uiStatusBarStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#5c6166", Dark: "#bfbdb6"}).
				Background(lipgloss.AdaptiveColor{Light: "#e7e8e9", Dark: "#2b2d30"}).
				Padding(0, 1)

	uiSelectedStyle = lipgloss.NewStyle().
			Background(lipgloss.AdaptiveColor{Light: "#d1e4f6", Dark: "#2a4054"}).
			Foreground(lipgloss.AdaptiveColor{Light: "#0d1017", Dark: "#e6e1cf"})

	uiNormalStyle = lipgloss.NewStyle()

	uiHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#828c99", Dark: "#6c7680"})

	uiSearchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#399ee6", Dark: "#59c2ff"}).
			Bold(true)
)

// --- View Mode ---

type viewMode int

const (
	viewList viewMode = iota
	viewDetail
)

// --- Status Filter ---

type statusFilter int

const (
	filterAll statusFilter = iota
	filterOpen
	filterInProgress
	filterBlocked
	filterClosed
)

func (f statusFilter) String() string {
	switch f {
	case filterOpen:
		return "open"
	case filterInProgress:
		return "in_progress"
	case filterBlocked:
		return "blocked"
	case filterClosed:
		return "closed"
	default:
		return "all"
	}
}

func (f statusFilter) toTypesStatus() *types.Status {
	switch f {
	case filterOpen:
		s := types.StatusOpen
		return &s
	case filterInProgress:
		s := types.StatusInProgress
		return &s
	case filterBlocked:
		s := types.StatusBlocked
		return &s
	case filterClosed:
		s := types.StatusClosed
		return &s
	default:
		return nil
	}
}

// --- Messages ---

type issuesLoadedMsg struct {
	issues []*types.Issue
	err    error
}

type issueDetailMsg struct {
	issue    *types.Issue
	labels   []string
	comments []*types.Comment
	deps     []*types.IssueWithDependencyMetadata
	children []*types.IssueWithDependencyMetadata
	err      error
}

// --- Key Map ---

type uiKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Back     key.Binding
	Quit     key.Binding
	Search   key.Binding
	Filter   key.Binding
	Refresh  key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding
}

var uiKeys = uiKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "view"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "backspace"),
		key.WithHelp("esc", "back"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Filter: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "filter"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup", "ctrl+u"),
		key.WithHelp("pgup", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown", "ctrl+d"),
		key.WithHelp("pgdn", "page down"),
	),
	Home: key.NewBinding(
		key.WithKeys("home", "g"),
		key.WithHelp("g/home", "top"),
	),
	End: key.NewBinding(
		key.WithKeys("end", "G"),
		key.WithHelp("G/end", "bottom"),
	),
}

// --- Model ---

type uiModel struct {
	// Data
	allIssues      []*types.Issue // unfiltered
	filteredIssues []*types.Issue // after filter + search
	selectedIssue  *types.Issue
	issueLabels    []string
	issueComments  []*types.Comment
	issueDeps      []*types.IssueWithDependencyMetadata
	issueChildren  []*types.IssueWithDependencyMetadata

	// State
	mode         viewMode
	cursor       int
	offset       int // scroll offset for list
	filter       statusFilter
	searching    bool
	searchInput  textinput.Model
	searchQuery  string
	detailVP     viewport.Model
	loading      bool
	loadingMsg   string
	err          error
	width        int
	height       int
	listHeight   int // computed from height minus chrome

	// Store reference
	store *dolt.DoltStore
}

func initialModel(s *dolt.DoltStore) uiModel {
	ti := textinput.New()
	ti.Placeholder = "Search issues..."
	ti.CharLimit = 100

	vp := viewport.New(80, 20)

	return uiModel{
		store:       s,
		searchInput: ti,
		detailVP:    vp,
		loading:     true,
		loadingMsg:  "Loading issues...",
		width:       80,
		height:      24,
		listHeight:  20,
	}
}

func (m uiModel) Init() tea.Cmd {
	return m.loadIssues()
}

// --- Commands ---

func (m uiModel) loadIssues() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		filter := types.IssueFilter{}
		if s := m.filter.toTypesStatus(); s != nil {
			filter.Status = s
		}

		issues, err := m.store.SearchIssues(ctx, m.searchQuery, filter)
		if err != nil {
			return issuesLoadedMsg{err: err}
		}

		// Sort: priority asc, then status (open > in_progress > blocked > closed), then updated desc
		sort.Slice(issues, func(i, j int) bool {
			a, b := issues[i], issues[j]
			// Open/in_progress first
			aActive := a.Status != types.StatusClosed
			bActive := b.Status != types.StatusClosed
			if aActive != bActive {
				return aActive
			}
			if a.Priority != b.Priority {
				return a.Priority < b.Priority
			}
			return a.UpdatedAt.After(b.UpdatedAt)
		})

		return issuesLoadedMsg{issues: issues}
	}
}

func (m uiModel) loadIssueDetail(id string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		issue, err := m.store.GetIssue(ctx, id)
		if err != nil {
			return issueDetailMsg{err: err}
		}
		if issue == nil {
			return issueDetailMsg{err: fmt.Errorf("issue not found: %s", id)}
		}

		labels, _ := m.store.GetLabels(ctx, id)
		comments, _ := m.store.GetIssueComments(ctx, id)
		deps, _ := m.store.GetDependenciesWithMetadata(ctx, id)
		children, _ := m.store.GetDependentsWithMetadata(ctx, id)

		return issueDetailMsg{
			issue:    issue,
			labels:   labels,
			comments: comments,
			deps:     deps,
			children: children,
		}
	}
}

// --- Update ---

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.listHeight = m.height - 4 // title + status bar + help + border
		if m.listHeight < 1 {
			m.listHeight = 1
		}
		m.detailVP.Width = m.width
		m.detailVP.Height = m.height - 3 // title + status bar + help
		return m, nil

	case issuesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.allIssues = msg.issues
		m.applyFilter()
		return m, nil

	case issueDetailMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.selectedIssue = msg.issue
		m.issueLabels = msg.labels
		m.issueComments = msg.comments
		m.issueDeps = msg.deps
		m.issueChildren = msg.children
		m.mode = viewDetail
		m.detailVP.SetContent(m.renderDetailContent())
		m.detailVP.GotoTop()
		return m, nil

	case tea.KeyMsg:
		// Handle search input mode
		if m.searching {
			return m.updateSearch(msg)
		}

		// Handle detail view
		if m.mode == viewDetail {
			return m.updateDetail(msg)
		}

		// Handle list view
		return m.updateList(msg)
	}

	return m, nil
}

func (m uiModel) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		m.searching = false
		m.searchQuery = m.searchInput.Value()
		m.loading = true
		m.loadingMsg = "Searching..."
		return m, m.loadIssues()

	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		m.searching = false
		m.searchInput.SetValue(m.searchQuery)
		return m, nil

	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
}

func (m uiModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, uiKeys.Quit):
		return m, tea.Quit

	case key.Matches(msg, uiKeys.Up):
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.offset {
				m.offset = m.cursor
			}
		}
		return m, nil

	case key.Matches(msg, uiKeys.Down):
		if m.cursor < len(m.filteredIssues)-1 {
			m.cursor++
			if m.cursor >= m.offset+m.listHeight {
				m.offset = m.cursor - m.listHeight + 1
			}
		}
		return m, nil

	case key.Matches(msg, uiKeys.PageUp):
		m.cursor -= m.listHeight
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.offset = m.cursor
		return m, nil

	case key.Matches(msg, uiKeys.PageDown):
		m.cursor += m.listHeight
		if m.cursor >= len(m.filteredIssues) {
			m.cursor = len(m.filteredIssues) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.offset = m.cursor - m.listHeight + 1
		if m.offset < 0 {
			m.offset = 0
		}
		return m, nil

	case key.Matches(msg, uiKeys.Home):
		m.cursor = 0
		m.offset = 0
		return m, nil

	case key.Matches(msg, uiKeys.End):
		m.cursor = len(m.filteredIssues) - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.offset = m.cursor - m.listHeight + 1
		if m.offset < 0 {
			m.offset = 0
		}
		return m, nil

	case key.Matches(msg, uiKeys.Enter):
		if len(m.filteredIssues) > 0 && m.cursor < len(m.filteredIssues) {
			issue := m.filteredIssues[m.cursor]
			m.loading = true
			m.loadingMsg = "Loading issue..."
			return m, m.loadIssueDetail(issue.ID)
		}
		return m, nil

	case key.Matches(msg, uiKeys.Search):
		m.searching = true
		m.searchInput.Focus()
		return m, textinput.Blink

	case key.Matches(msg, uiKeys.Filter):
		m.filter = (m.filter + 1) % 5
		m.cursor = 0
		m.offset = 0
		m.loading = true
		m.loadingMsg = "Filtering..."
		return m, m.loadIssues()

	case key.Matches(msg, uiKeys.Refresh):
		m.loading = true
		m.loadingMsg = "Refreshing..."
		return m, m.loadIssues()
	}

	return m, nil
}

func (m uiModel) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, uiKeys.Quit):
		return m, tea.Quit

	case key.Matches(msg, uiKeys.Back):
		m.mode = viewList
		m.selectedIssue = nil
		return m, nil

	default:
		var cmd tea.Cmd
		m.detailVP, cmd = m.detailVP.Update(msg)
		return m, cmd
	}
}

// --- Filter ---

func (m *uiModel) applyFilter() {
	m.filteredIssues = m.allIssues
	m.cursor = 0
	m.offset = 0
}

// --- View ---

func (m uiModel) View() string {
	if m.loading {
		return m.renderLoading()
	}
	if m.err != nil {
		return m.renderError()
	}

	switch m.mode {
	case viewDetail:
		return m.renderDetail()
	default:
		return m.renderList()
	}
}

func (m uiModel) renderLoading() string {
	return fmt.Sprintf("\n  %s\n", m.loadingMsg)
}

func (m uiModel) renderError() string {
	return fmt.Sprintf("\n  Error: %v\n\n  Press q to quit.\n", m.err)
}

func (m uiModel) renderList() string {
	var b strings.Builder

	// Title bar
	title := uiTitleStyle.Render("bd ui")
	filterTag := ""
	if m.filter != filterAll {
		filterTag = fmt.Sprintf(" [%s]", m.filter.String())
	}
	searchTag := ""
	if m.searchQuery != "" {
		searchTag = fmt.Sprintf(" search:%q", m.searchQuery)
	}
	countTag := fmt.Sprintf(" (%d issues)", len(m.filteredIssues))
	b.WriteString(title + uiHelpStyle.Render(filterTag+searchTag+countTag) + "\n")

	// Search input (if active)
	if m.searching {
		b.WriteString(uiSearchStyle.Render("/") + " ")
		b.WriteString(m.searchInput.View())
		b.WriteString("\n")
	}

	// Issue list
	if len(m.filteredIssues) == 0 {
		b.WriteString("\n  No issues found.\n")
	} else {
		end := m.offset + m.listHeight
		if end > len(m.filteredIssues) {
			end = len(m.filteredIssues)
		}
		for i := m.offset; i < end; i++ {
			issue := m.filteredIssues[i]
			line := m.formatListItem(issue)
			if i == m.cursor {
				// Pad the selected line to full width for highlight
				padding := m.width - lipgloss.Width(line)
				if padding > 0 {
					line = line + strings.Repeat(" ", padding)
				}
				line = uiSelectedStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}
	}

	// Pad to fill height
	rendered := strings.Count(b.String(), "\n")
	headerLines := 1
	if m.searching {
		headerLines = 2
	}
	target := m.listHeight + headerLines
	for rendered < target {
		b.WriteString("\n")
		rendered++
	}

	// Status bar
	b.WriteString(m.renderStatusBar())

	return b.String()
}

func (m uiModel) formatListItem(issue *types.Issue) string {
	statusIcon := ui.GetStatusIcon(string(issue.Status))
	prio := fmt.Sprintf("P%d", issue.Priority)

	typeBadge := ""
	switch issue.IssueType {
	case "epic":
		typeBadge = "[epic] "
	case "bug":
		typeBadge = "[bug] "
	}

	assignee := ""
	if issue.Assignee != "" {
		assignee = " @" + issue.Assignee
	}

	line := fmt.Sprintf(" %s %s %s %s%s%s",
		statusIcon, issue.ID, prio, typeBadge, issue.Title, assignee)

	// Apply status-based styling
	if !ui.ShouldUseColor() {
		return line
	}

	switch issue.Status {
	case types.StatusClosed:
		return ui.StatusClosedStyle.Render(line)
	default:
		// Color the priority based on level
		styledStatus := ui.RenderStatusIcon(string(issue.Status))
		styledPrio := ui.RenderPriorityCompact(issue.Priority)
		styledType := ""
		switch issue.IssueType {
		case "epic":
			styledType = ui.TypeEpicStyle.Render("[epic]") + " "
		case "bug":
			styledType = ui.TypeBugStyle.Render("[bug]") + " "
		}
		styledAssignee := ""
		if issue.Assignee != "" {
			styledAssignee = " " + ui.RenderMuted("@"+issue.Assignee)
		}

		return fmt.Sprintf(" %s %s %s %s%s%s",
			styledStatus, issue.ID, styledPrio, styledType, issue.Title, styledAssignee)
	}
}

func (m uiModel) renderStatusBar() string {
	left := fmt.Sprintf(" %d/%d", m.cursor+1, len(m.filteredIssues))
	if len(m.filteredIssues) == 0 {
		left = " 0/0"
	}

	right := " j/k:navigate  enter:view  f:filter  /:search  r:refresh  q:quit "
	if m.mode == viewDetail {
		right = " j/k:scroll  esc:back  q:quit "
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	bar := left + strings.Repeat(" ", gap) + right

	return uiStatusBarStyle.Render(bar)
}

func (m uiModel) renderDetail() string {
	var b strings.Builder

	// Title bar
	if m.selectedIssue != nil {
		title := uiTitleStyle.Render(m.selectedIssue.ID + " · " + m.selectedIssue.Title)
		b.WriteString(title + "\n")
	}

	// Viewport with detail content
	b.WriteString(m.detailVP.View() + "\n")

	// Status bar
	scrollPct := fmt.Sprintf(" %d%%", int(m.detailVP.ScrollPercent()*100))
	right := " j/k:scroll  esc:back  q:quit "
	gap := m.width - lipgloss.Width(scrollPct) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	bar := scrollPct + strings.Repeat(" ", gap) + right
	b.WriteString(uiStatusBarStyle.Render(bar))

	return b.String()
}

func (m uiModel) renderDetailContent() string {
	issue := m.selectedIssue
	if issue == nil {
		return "No issue selected."
	}

	var b strings.Builder

	// Header
	statusIcon := ui.RenderStatusIcon(string(issue.Status))
	statusStr := strings.ToUpper(string(issue.Status))
	priorityStr := fmt.Sprintf("P%d", issue.Priority)
	b.WriteString(fmt.Sprintf(" %s %s  %s  %s\n", statusIcon, statusStr, priorityStr, string(issue.IssueType)))
	b.WriteString(" " + ui.SeparatorLight + "\n")

	// Metadata
	if issue.Assignee != "" {
		b.WriteString(fmt.Sprintf(" Assignee: %s\n", issue.Assignee))
	}
	if issue.CreatedBy != "" {
		b.WriteString(fmt.Sprintf(" Owner:    %s\n", issue.CreatedBy))
	}
	b.WriteString(fmt.Sprintf(" Created:  %s\n", issue.CreatedAt.Format("2006-01-02 15:04")))
	b.WriteString(fmt.Sprintf(" Updated:  %s\n", issue.UpdatedAt.Format("2006-01-02 15:04")))
	if issue.ClosedAt != nil {
		b.WriteString(fmt.Sprintf(" Closed:   %s\n", issue.ClosedAt.Format("2006-01-02 15:04")))
	}
	if issue.DueAt != nil {
		b.WriteString(fmt.Sprintf(" Due:      %s\n", issue.DueAt.Format("2006-01-02 15:04")))
	}

	// Labels
	if len(m.issueLabels) > 0 {
		b.WriteString(fmt.Sprintf("\n Labels: %s\n", strings.Join(m.issueLabels, ", ")))
	}

	// Description
	if issue.Description != "" {
		b.WriteString("\n DESCRIPTION\n")
		b.WriteString(" " + ui.SeparatorLight + "\n")
		for _, line := range strings.Split(issue.Description, "\n") {
			b.WriteString(" " + line + "\n")
		}
	}

	// Design
	if issue.Design != "" {
		b.WriteString("\n DESIGN\n")
		b.WriteString(" " + ui.SeparatorLight + "\n")
		for _, line := range strings.Split(issue.Design, "\n") {
			b.WriteString(" " + line + "\n")
		}
	}

	// Notes
	if issue.Notes != "" {
		b.WriteString("\n NOTES\n")
		b.WriteString(" " + ui.SeparatorLight + "\n")
		for _, line := range strings.Split(issue.Notes, "\n") {
			b.WriteString(" " + line + "\n")
		}
	}

	// Acceptance Criteria
	if issue.AcceptanceCriteria != "" {
		b.WriteString("\n ACCEPTANCE CRITERIA\n")
		b.WriteString(" " + ui.SeparatorLight + "\n")
		for _, line := range strings.Split(issue.AcceptanceCriteria, "\n") {
			b.WriteString(" " + line + "\n")
		}
	}

	// Dependencies
	if len(m.issueDeps) > 0 {
		b.WriteString("\n DEPENDS ON\n")
		b.WriteString(" " + ui.SeparatorLight + "\n")
		for _, dep := range m.issueDeps {
			statusIcon := ui.GetStatusIcon(string(dep.Status))
			b.WriteString(fmt.Sprintf("  %s %s %s (%s)\n",
				statusIcon, dep.ID, dep.Title, dep.DependencyType))
		}
	}

	// Children
	if len(m.issueChildren) > 0 {
		hasChildren := false
		for _, dep := range m.issueChildren {
			if dep.DependencyType == types.DepParentChild {
				if !hasChildren {
					b.WriteString("\n CHILDREN\n")
					b.WriteString(" " + ui.SeparatorLight + "\n")
					hasChildren = true
				}
				statusIcon := ui.GetStatusIcon(string(dep.Status))
				b.WriteString(fmt.Sprintf("  %s %s %s\n", statusIcon, dep.ID, dep.Title))
			}
		}
	}

	// Comments
	if len(m.issueComments) > 0 {
		b.WriteString("\n COMMENTS\n")
		b.WriteString(" " + ui.SeparatorLight + "\n")
		for _, comment := range m.issueComments {
			ts := comment.CreatedAt.UTC().Format("2006-01-02 15:04")
			b.WriteString(fmt.Sprintf("  %s  %s\n", ts, comment.Author))
			for _, line := range strings.Split(comment.Text, "\n") {
				b.WriteString("    " + line + "\n")
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// --- Cobra Command ---

var uiCmd = &cobra.Command{
	Use:     "ui",
	GroupID: "views",
	Short:   "Interactive terminal UI for browsing issues",
	Long: `Launch an interactive terminal interface for browsing and viewing issues.

Navigate with vim-style keys (j/k) or arrow keys. Press Enter to view
issue details, / to search, f to cycle status filters, and q to quit.

Keyboard shortcuts:
  j/k, ↑/↓     Navigate issue list
  Enter         View issue details
  Esc           Back to list
  /             Search issues
  f             Cycle status filter (all → open → in_progress → blocked → closed)
  r             Refresh issue list
  g/Home        Jump to top
  G/End         Jump to bottom
  PgUp/PgDn     Page up/down
  q, Ctrl+C     Quit

Examples:
  bd ui                  # Launch interactive issue browser`,
	Run: func(cmd *cobra.Command, _ []string) {
		if err := ensureDirectMode("ui command requires database access"); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		s := getStore()
		if s == nil {
			fmt.Fprintf(os.Stderr, "Error: no database available\n")
			os.Exit(1)
		}

		m := initialModel(s)
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
