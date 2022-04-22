package shared

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
)

func StateTitleWithColor(cs *iostreams.ColorScheme, pr api.PullRequest) string {
	prStateColorFunc := cs.ColorFromString(ColorForPRState(pr))

	if pr.State == "OPEN" && pr.IsDraft {
		return prStateColorFunc(strings.Title(strings.ToLower("Draft")))
	}
	return prStateColorFunc(strings.Title(strings.ToLower(pr.State)))
}

func ColorForPRState(pr api.PullRequest) string {
	switch pr.State {
	case "OPEN":
		if pr.IsDraft {
			return "gray"
		}
		return "green"
	case "CLOSED":
		return "red"
	case "MERGED":
		return "magenta"
	default:
		return ""
	}
}

func ColorForIssueState(issue api.Issue) string {
	switch issue.State {
	case "OPEN":
		return "green"
	case "CLOSED":
		return "magenta"
	default:
		return ""
	}
}

func PrintHeader(io *iostreams.IOStreams, s string) {
	fmt.Fprintln(io.Out, io.ColorScheme().Bold(s))
}

func PrintMessage(io *iostreams.IOStreams, s string) {
	fmt.Fprintln(io.Out, io.ColorScheme().Gray(s))
}

func ListNoResults(io *iostreams.IOStreams, repoName string, itemName string, hasFilters bool) error {
	if hasFilters {
		return cmdutil.NoResultsError(io, fmt.Sprintf("No %ss match your search in %s", itemName, repoName))
	}
	return cmdutil.NoResultsError(io, fmt.Sprintf("There are no open %ss in %s", itemName, repoName))
}

func ListHeader(repoName string, itemName string, matchCount int, totalMatchCount int, hasFilters bool) string {
	if totalMatchCount == 0 {
		if hasFilters {
			return fmt.Sprintf("No %ss match your search in %s", itemName, repoName)
		}
		return fmt.Sprintf("There are no open %ss in %s", itemName, repoName)
	}

	if hasFilters {
		matchVerb := "match"
		if totalMatchCount == 1 {
			matchVerb = "matches"
		}
		return fmt.Sprintf("Showing %d of %s in %s that %s your search", matchCount, utils.Pluralize(totalMatchCount, itemName), repoName, matchVerb)
	}

	return fmt.Sprintf("Showing %d of %s in %s", matchCount, utils.Pluralize(totalMatchCount, fmt.Sprintf("open %s", itemName)), repoName)
}
