package main

import (
	"fmt"
	"io"
	"os"

	"github.com/steveyegge/beads/internal/tracker"
	"github.com/steveyegge/beads/internal/ui"
)

// configureSyncUI wires standard sync progress callbacks for tracker commands.
func configureSyncUI(engine *tracker.Engine, out io.Writer) {
	engine.OnMessage = func(msg string) {
		_, _ = fmt.Fprintf(out, "  %s %s\n", ui.RenderInfoIcon(), msg)
	}
	engine.OnWarning = func(msg string) {
		_, _ = fmt.Fprintf(os.Stderr, "%s %s\n", ui.RenderWarnIcon(), msg)
	}
}

// printSyncSummary renders a consistent human-readable sync summary.
func printSyncSummary(out io.Writer, provider string, result *tracker.SyncResult, dryRun bool) {
	if dryRun {
		_, _ = fmt.Fprintf(out, "\n%s Dry run complete (no changes made)\n", ui.RenderPassIcon())
		return
	}

	if result.Stats.Pulled > 0 {
		_, _ = fmt.Fprintf(out, "%s Pulled %d issues (%d created, %d updated)\n",
			ui.RenderPassIcon(), result.Stats.Pulled, result.Stats.Created, result.Stats.Updated)
	}
	if result.Stats.Pushed > 0 {
		_, _ = fmt.Fprintf(out, "%s Pushed %d issues\n", ui.RenderPassIcon(), result.Stats.Pushed)
	}
	if result.Stats.Conflicts > 0 {
		_, _ = fmt.Fprintf(out, "%s Resolved %d conflicts\n", ui.RenderInfoIcon(), result.Stats.Conflicts)
	}

	_, _ = fmt.Fprintf(out, "\n%s %s sync complete\n", ui.RenderPassIcon(), provider)

	if len(result.Warnings) == 0 {
		return
	}

	_, _ = fmt.Fprintf(out, "\n%s Warnings:\n", ui.RenderWarnIcon())
	for _, w := range result.Warnings {
		_, _ = fmt.Fprintf(out, "  - %s\n", w)
	}
}
