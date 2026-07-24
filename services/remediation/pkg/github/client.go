package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v62/github"
	"golang.org/x/oauth2"
)

// Client wraps the GitHub API for opening remediation PRs
type Client struct {
	gh    *github.Client
	owner string
	repo  string
}

// NewClient creates an authenticated GitHub client
func NewClient(token, owner, repo string) *Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	return &Client{gh: github.NewClient(tc), owner: owner, repo: repo}
}

// RemediationPR describes the pull request to open
type RemediationPR struct {
	BranchName  string
	Title       string
	Body        string
	FilePath    string
	FileContent string
	CommitMsg   string
}

// OpenRemediationPR creates a branch, commits the fix, and opens a PR.
// Returns the PR URL and number.
func (c *Client) OpenRemediationPR(ctx context.Context, pr RemediationPR) (string, int, error) {
	// Get default branch ref
	baseRef, _, err := c.gh.Git.GetRef(ctx, c.owner, c.repo, "refs/heads/main")
	if err != nil {
		return "", 0, fmt.Errorf("get base ref: %w", err)
	}

	// Create new branch
	newRef := &github.Reference{
		Ref:    github.String("refs/heads/" + pr.BranchName),
		Object: &github.GitObject{SHA: baseRef.Object.SHA},
	}
	_, _, err = c.gh.Git.CreateRef(ctx, c.owner, c.repo, newRef)
	if err != nil {
		return "", 0, fmt.Errorf("create branch: %w", err)
	}

	// Get current file content (to know its SHA for update)
	fileContent, _, _, err := c.gh.Repositories.GetContents(
		ctx, c.owner, c.repo, pr.FilePath,
		&github.RepositoryContentGetOptions{Ref: pr.BranchName},
	)

	opts := &github.RepositoryContentFileOptions{
		Message: github.String(pr.CommitMsg),
		Content: []byte(pr.FileContent),
		Branch:  github.String(pr.BranchName),
	}
	if err == nil && fileContent != nil {
		opts.SHA = fileContent.SHA
	}

	_, _, err = c.gh.Repositories.UpdateFile(ctx, c.owner, c.repo, pr.FilePath, opts)
	if err != nil {
		return "", 0, fmt.Errorf("commit fix: %w", err)
	}

	// Open the pull request
	newPR := &github.NewPullRequest{
		Title: github.String(pr.Title),
		Head:  github.String(pr.BranchName),
		Base:  github.String("main"),
		Body:  github.String(pr.Body),
	}
	created, _, err := c.gh.PullRequests.Create(ctx, c.owner, c.repo, newPR)
	if err != nil {
		return "", 0, fmt.Errorf("create PR: %w", err)
	}

	return created.GetHTMLURL(), created.GetNumber(), nil
}
