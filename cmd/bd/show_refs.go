package main

import (
	"context"
	"fmt"
	"os"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/ui"
)

// showIssueRefs displays issues that reference the given issue(s), grouped by relationship type
func showIssueRefs(ctx context.Context, args []string, jsonOut bool) {
	// Collect all refs for all issues
	allRefs := make(map[string][]*types.IssueWithDependencyMetadata)

	// Process each issue
	processIssue := func(issueID string, issueStore *dolt.DoltStore) error {
		refs, err := issueStore.GetDependentsWithMetadata(ctx, issueID)
		if err != nil {
			return err
		}
		allRefs[issueID] = refs
		return nil
	}

	// Process each arg via routing-aware resolution
	for _, id := range args {
		result, err := resolveAndGetIssueWithRouting(ctx, store, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving %s: %v\n", id, err)
			continue
		}
		if result == nil || result.Issue == nil {
			if result != nil {
				result.Close()
			}
			fmt.Fprintf(os.Stderr, "Issue %s not found\n", id)
			continue
		}
		if err := processIssue(result.ResolvedID, result.Store); err != nil {
			fmt.Fprintf(os.Stderr, "Error getting refs for %s: %v\n", id, err)
		}
		result.Close()
	}

	// Output results
	if jsonOut {
		outputJSON(allRefs)
		return
	}

	// Display refs grouped by issue and relationship type
	for issueID, refs := range allRefs {
		if len(refs) == 0 {
			fmt.Printf("\n%s: No references found\n", ui.RenderAccent(issueID))
			continue
		}

		fmt.Printf("\n%s References to %s:\n", ui.RenderInfoIcon(), issueID)

		// Group refs by type
		refsByType := make(map[types.DependencyType][]*types.IssueWithDependencyMetadata)
		for _, ref := range refs {
			refsByType[ref.DependencyType] = append(refsByType[ref.DependencyType], ref)
		}

		// Display each type
		typeOrder := []types.DependencyType{
			types.DepUntil, types.DepCausedBy, types.DepValidates,
			types.DepBlocks, types.DepParentChild, types.DepRelatesTo,
			types.DepTracks, types.DepDiscoveredFrom, types.DepRelated,
			types.DepSupersedes, types.DepDuplicates, types.DepRepliesTo,
			types.DepApprovedBy, types.DepAuthoredBy, types.DepAssignedTo,
		}

		// First show types in order, then any others
		shown := make(map[types.DependencyType]bool)
		for _, depType := range typeOrder {
			if refs, ok := refsByType[depType]; ok {
				displayRefGroup(depType, refs)
				shown[depType] = true
			}
		}
		// Show any remaining types
		for depType, refs := range refsByType {
			if !shown[depType] {
				displayRefGroup(depType, refs)
			}
		}
		fmt.Println()
	}
}

// displayRefGroup displays a group of references with a given type
// Closed items get entire row muted - the work is done, no need for attention
func displayRefGroup(depType types.DependencyType, refs []*types.IssueWithDependencyMetadata) {
	icon := getRefTypeIcon(depType)
	fmt.Printf("\n  %s %s (%d):\n", icon, depType, len(refs))

	for _, ref := range refs {
		// Closed items: mute entire row since the work is complete
		if ref.Status == types.StatusClosed {
			fmt.Printf("    %s: %s %s\n",
				ui.RenderMuted(ref.ID),
				ui.RenderMuted(ref.Title),
				ui.RenderMuted(fmt.Sprintf("[P%d - %s]", ref.Priority, ref.Status)))
			continue
		}

		// Active items: color ID based on status
		var idStr string
		switch ref.Status {
		case types.StatusOpen:
			idStr = ui.StatusOpenStyle.Render(ref.ID)
		case types.StatusInProgress:
			idStr = ui.StatusInProgressStyle.Render(ref.ID)
		case types.StatusBlocked:
			idStr = ui.StatusBlockedStyle.Render(ref.ID)
		default:
			idStr = ref.ID
		}
		fmt.Printf("    %s: %s [P%d - %s]\n", idStr, ref.Title, ref.Priority, ref.Status)
	}
}

// getRefTypeIcon returns a compact symbol for a dependency/reference type.
// Keep these as small Unicode symbols (not emoji decorations) for consistency.
func getRefTypeIcon(depType types.DependencyType) string {
	switch depType {
	case types.DepUntil:
		return "⧖" // waiting until
	case types.DepCausedBy:
		return "↯" // triggered by
	case types.DepValidates:
		return "✓" // validates
	case types.DepBlocks:
		return "⊘" // blocks
	case types.DepParentChild:
		return "↳" // Child arrow
	case types.DepRelatesTo, types.DepRelated:
		return "↔" // Bidirectional
	case types.DepTracks:
		return "≋" // tracks
	case types.DepDiscoveredFrom:
		return "◊" // Diamond - discovered
	case types.DepSupersedes:
		return "↑" // supersedes
	case types.DepDuplicates:
		return "≡" // duplicates
	case types.DepRepliesTo:
		return "↩" // replies to
	case types.DepApprovedBy:
		return "+" // approved
	case types.DepAuthoredBy:
		return "✎" // authored
	case types.DepAssignedTo:
		return "@" // assigned
	default:
		return "→" // Default arrow
	}
}
