package edit

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	shared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

type EditOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	DetermineEditor    func() (string, error)
	FieldsToEditSurvey func(*prShared.Editable) error
	EditFieldsSurvey   func(*prShared.Editable, string) error
	FetchOptions       func(*api.Client, ghrepo.Interface, *prShared.Editable) error

	SelectorArgs []string
	Interactive  bool

	workers int

	prShared.Editable
}

func NewCmdEdit(f *cmdutil.Factory, runF func(*EditOptions) error) *cobra.Command {
	opts := &EditOptions{
		IO:                 f.IOStreams,
		HttpClient:         f.HttpClient,
		DetermineEditor:    func() (string, error) { return cmdutil.DetermineEditor(f.Config) },
		FieldsToEditSurvey: prShared.FieldsToEditSurvey,
		EditFieldsSurvey:   prShared.EditFieldsSurvey,
		FetchOptions:       prShared.FetchOptions,
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "edit {<numbers> | <urls>}",
		Short: "Edit issues",
		Long: heredoc.Doc(`
			Edit one or more issues within the same repository.

			Editing issues' projects requires authorization with the "project" scope.
			To authorize, run "gh auth refresh -s project".
		`),
		Example: heredoc.Doc(`
			$ gh issue edit 23 --title "I found a bug" --body "Nothing works"
			$ gh issue edit 23 --add-label "bug,help wanted" --remove-label "core"
			$ gh issue edit 23 --add-assignee "@me" --remove-assignee monalisa,hubot
			$ gh issue edit 23 --add-project "Roadmap" --remove-project v1,v2
			$ gh issue edit 23 --milestone "Version 1"
			$ gh issue edit 23 --body-file body.txt
			$ gh issue edit 23 34 --add-label "help wanted"
		`),
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.SelectorArgs = args

			flags := cmd.Flags()

			bodyProvided := flags.Changed("body")
			bodyFileProvided := bodyFile != ""

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--body` or `--body-file`",
				bodyProvided,
				bodyFileProvided,
			); err != nil {
				return err
			}
			if bodyProvided || bodyFileProvided {
				opts.Editable.Body.Edited = true
				if bodyFileProvided {
					b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
					if err != nil {
						return err
					}
					opts.Editable.Body.Value = string(b)
				}
			}

			if flags.Changed("title") {
				opts.Editable.Title.Edited = true
			}
			if flags.Changed("add-assignee") || flags.Changed("remove-assignee") {
				opts.Editable.Assignees.Edited = true
			}
			if flags.Changed("add-label") || flags.Changed("remove-label") {
				opts.Editable.Labels.Edited = true
			}
			if flags.Changed("add-project") || flags.Changed("remove-project") {
				opts.Editable.Projects.Edited = true
			}
			if flags.Changed("milestone") {
				opts.Editable.Milestone.Edited = true
			}

			if !opts.Editable.Dirty() {
				opts.Interactive = true
			}

			if opts.Interactive && !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("field to edit flag required when not running interactively")
			}

			if opts.Interactive && len(opts.SelectorArgs) > 1 {
				return cmdutil.FlagErrorf("multiple issues cannot be edited interactively")
			}

			if runF != nil {
				return runF(opts)
			}

			return editRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Editable.Title.Value, "title", "t", "", "Set the new title.")
	cmd.Flags().StringVarP(&opts.Editable.Body.Value, "body", "b", "", "Set the new body.")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	cmd.Flags().StringSliceVar(&opts.Editable.Assignees.Add, "add-assignee", nil, "Add assigned users by their `login`. Use \"@me\" to assign yourself.")
	cmd.Flags().StringSliceVar(&opts.Editable.Assignees.Remove, "remove-assignee", nil, "Remove assigned users by their `login`. Use \"@me\" to unassign yourself.")
	cmd.Flags().StringSliceVar(&opts.Editable.Labels.Add, "add-label", nil, "Add labels by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Labels.Remove, "remove-label", nil, "Remove labels by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Projects.Add, "add-project", nil, "Add the issue to projects by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Projects.Remove, "remove-project", nil, "Remove the issue from projects by `name`")
	cmd.Flags().StringVarP(&opts.Editable.Milestone.Value, "milestone", "m", "", "Edit the milestone the issue belongs to by `name`")

	return cmd
}

func editRun(opts *EditOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	// Prompt the user which fields they'd like to edit.
	editable := opts.Editable
	if opts.Interactive {
		err = opts.FieldsToEditSurvey(&editable)
		if err != nil {
			return err
		}
	}

	lookupFields := []string{"id", "number", "title", "body", "url"}
	if editable.Assignees.Edited {
		lookupFields = append(lookupFields, "assignees")
	}
	if editable.Labels.Edited {
		lookupFields = append(lookupFields, "labels")
	}
	if editable.Projects.Edited {
		lookupFields = append(lookupFields, "projectCards")
		lookupFields = append(lookupFields, "projectItems")
	}
	if editable.Milestone.Edited {
		lookupFields = append(lookupFields, "milestone")
	}

	// Get all specified issues and make sure they are within the same repo.
	issues, repo, err := shared.IssuesFromArgsWithFields(httpClient, opts.BaseRepo, opts.SelectorArgs, lookupFields)
	if err != nil {
		return err
	}

	// Fetch editable shared fields once for all issues.
	apiClient := api.NewClientFromHTTP(httpClient)
	opts.IO.StartProgressIndicatorWithLabel("Fetching repository information")
	err = opts.FetchOptions(apiClient, repo, &editable)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	// Update all issues in parallel.
	issueURLs := make(chan string, len(issues))
	if opts.workers == 0 {
		opts.workers = 10
	}
	g := errgroup.Group{}
	g.SetLimit(opts.workers)

	opts.IO.StartProgressIndicatorWithLabel(fmt.Sprintf("Updating %d issues", len(issues)))
	for i := range issues {
		// Copy variables to capture in the go routine below.
		issue := issues[i]
		editable := editable.Clone()

		g.Go(func() error {
			editable.Title.Default = issue.Title
			editable.Body.Default = issue.Body
			editable.Assignees.Default = issue.Assignees.Logins()
			editable.Labels.Default = issue.Labels.Names()
			editable.Projects.Default = append(issue.ProjectCards.ProjectNames(), issue.ProjectItems.ProjectTitles()...)
			projectItems := map[string]string{}
			for _, n := range issue.ProjectItems.Nodes {
				projectItems[n.Project.ID] = n.ID
			}
			editable.Projects.ProjectItems = projectItems
			if issue.Milestone != nil {
				editable.Milestone.Default = issue.Milestone.Title
			}

			// Allow interactive prompts for one issue; failed earlier if multiple issues specified.
			if opts.Interactive {
				editorCommand, err := opts.DetermineEditor()
				if err != nil {
					return err
				}
				err = opts.EditFieldsSurvey(&editable, editorCommand)
				if err != nil {
					return err
				}
			}

			err = prShared.UpdateIssue(httpClient, repo, issue.ID, issue.IsPullRequest(), editable)
			if err != nil {
				return err
			}

			issueURLs <- issue.URL
			return nil
		})
	}
	opts.IO.StopProgressIndicator()

	err = g.Wait()
	close(issueURLs)

	if err != nil {
		return err
	}

	for issueURL := range issueURLs {
		fmt.Fprintln(opts.IO.Out, issueURL)
	}

	return nil
}
