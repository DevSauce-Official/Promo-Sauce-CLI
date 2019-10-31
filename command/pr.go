package command

import (
	"fmt"
	"strconv"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(prCmd)
	prCmd.AddCommand(prListCmd)
	prCmd.AddCommand(prStatusCmd)
	prCmd.AddCommand(prViewCmd)

	prListCmd.Flags().IntP("limit", "L", 30, "maximum number of items to fetch")
	prListCmd.Flags().StringP("state", "s", "open", "pull request state to filter by")
}

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Work with pull requests",
	Long:  `Helps you work with pull requests.`,
}
var prListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pull requests",
	RunE:  prList,
}
var prStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of relevant pull requests",
	RunE:  prStatus,
}
var prViewCmd = &cobra.Command{
	Use:   "view [pr-number]",
	Short: "Open a pull request in the browser",
	RunE:  prView,
}

func prStatus(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := ctx.BaseRepo()
	if err != nil {
		return err
	}
	currentBranch, err := ctx.Branch()
	if err != nil {
		return err
	}
	currentUser, err := ctx.AuthLogin()
	if err != nil {
		return err
	}

	prPayload, err := api.PullRequests(apiClient, baseRepo, currentBranch, currentUser)
	if err != nil {
		return err
	}

	printHeader("Current branch")
	if prPayload.CurrentPR != nil {
		printPrs(*prPayload.CurrentPR)
	} else {
		message := fmt.Sprintf("  There is no pull request associated with %s", utils.Cyan("["+currentBranch+"]"))
		printMessage(message)
	}
	fmt.Println()

	printHeader("Created by you")
	if len(prPayload.ViewerCreated) > 0 {
		printPrs(prPayload.ViewerCreated...)
	} else {
		printMessage("  You have no open pull requests")
	}
	fmt.Println()

	printHeader("Requesting a code review from you")
	if len(prPayload.ReviewRequested) > 0 {
		printPrs(prPayload.ReviewRequested...)
	} else {
		printMessage("  You have no pull requests to review")
	}
	fmt.Println()

	return nil
}

func prList(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := ctx.BaseRepo()
	if err != nil {
		return err
	}

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}
	state, err := cmd.Flags().GetString("state")
	if err != nil {
		return err
	}
	var graphqlState string
	switch state {
	case "open":
		graphqlState = "OPEN"
	case "closed":
		graphqlState = "CLOSED"
	case "all":
		graphqlState = "ALL"
	default:
		return fmt.Errorf("invalid state: %s", state)
	}

	params := map[string]interface{}{
		"owner": baseRepo.RepoOwner(),
		"repo":  baseRepo.RepoName(),
		"state": graphqlState,
	}

	prs, err := api.PullRequestList(apiClient, params, limit)
	if err != nil {
		return err
	}

	for _, pr := range prs {
		fmt.Printf("#%d\t%s\t%s\n", pr.Number, pr.Title, pr.HeadRefName)
	}
	return nil
}

func prView(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	baseRepo, err := ctx.BaseRepo()
	if err != nil {
		return err
	}

	var openURL string
	if len(args) > 0 {
		if prNumber, err := strconv.Atoi(args[0]); err == nil {
			// TODO: move URL generation into GitHubRepository
			openURL = fmt.Sprintf("https://github.com/%s/%s/pull/%d", baseRepo.RepoOwner(), baseRepo.RepoName(), prNumber)
		} else {
			return fmt.Errorf("invalid pull request number: '%s'", args[0])
		}
	} else {
		apiClient, err := apiClientForContext(ctx)
		if err != nil {
			return err
		}
		currentBranch, err := ctx.Branch()
		if err != nil {
			return err
		}

		prs, err := api.PullRequestsForBranch(apiClient, baseRepo, currentBranch)
		if err != nil {
			return err
		} else if len(prs) < 1 {
			return fmt.Errorf("the '%s' branch has no open pull requests", currentBranch)
		}
		openURL = prs[0].URL
	}

	fmt.Printf("Opening %s in your browser.\n", openURL)
	return utils.OpenInBrowser(openURL)
}

func printPrs(prs ...api.PullRequest) {
	for _, pr := range prs {
		fmt.Printf("  #%d %s %s\n", pr.Number, truncateTitle(pr.Title), utils.Cyan("["+pr.HeadRefName+"]"))
	}
}

func printHeader(s string) {
	fmt.Println(utils.Bold(s))
}

func printMessage(s string) {
	fmt.Println(utils.Gray(s))
}

func truncateTitle(title string) string {
	const maxLength = 50

	if len(title) > maxLength {
		return title[0:maxLength-3] + "..."
	}
	return title
}
