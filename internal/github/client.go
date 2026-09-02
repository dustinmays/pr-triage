package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/go-github/v72/github"
)

// Client wraps the GitHub API client.
type Client struct {
	gh *github.Client
}

// NewClient creates a GitHub client authenticated with the given PAT.
// Uses a 30-second HTTP timeout to prevent indefinite hangs on network issues.
// Wraps the transport with ETag caching so conditional requests (304 Not Modified)
// don't count against the GitHub API rate limit.
func NewClient(token string) *Client {
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: newETagTransport(http.DefaultTransport),
	}
	c := github.NewClient(httpClient)
	if token != "" {
		c = c.WithAuthToken(token)
	}
	return &Client{gh: c}
}

// NewClientWithBaseURL creates a GitHub client pointing at a custom API base URL.
// Used for testing with httptest servers.
func NewClientWithBaseURL(token, baseURL string) *Client {
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: newETagTransport(http.DefaultTransport),
	}
	c := github.NewClient(httpClient)
	if token != "" {
		c = c.WithAuthToken(token)
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	parsedURL, err := url.Parse(baseURL)
	if err == nil && parsedURL != nil {
		c.BaseURL = parsedURL
	}
	return &Client{gh: c}
}

// GH returns the underlying go-github client.
func (c *Client) GH() *github.Client {
	return c.gh
}

// CoreRateLimit returns the core API rate limit status for the client's token.
func (c *Client) CoreRateLimit(ctx context.Context) (*github.Rate, error) {
	limits, _, err := c.gh.RateLimit.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("get rate limits: %w", err)
	}
	return limits.GetCore(), nil
}

// ValidateToken confirms the client's credentials are accepted by GitHub and
// returns the authenticated user's login. It exists so callers like `pr-triage
// setup` can catch a bad, expired, or mistyped token immediately, instead of
// only discovering it later when it fails silently deep in the poll loop.
func (c *Client) ValidateToken(ctx context.Context) (string, error) {
	user, _, err := c.gh.Users.Get(ctx, "")
	if err != nil {
		return "", fmt.Errorf("validate token: %w", err)
	}
	return user.GetLogin(), nil
}

// ListOpenPRs lists open pull requests for a repository, optionally filtered by baseRef.
// baseRef can be an exact branch name (e.g. "main") or a glob pattern (e.g. "release/*").
// If baseRef is empty, all open PRs are returned.
func (c *Client) ListOpenPRs(ctx context.Context, owner, repo, baseRef string) ([]*github.PullRequest, error) {
	var allPRs []*github.PullRequest
	opts := &github.PullRequestListOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		prs, resp, err := c.gh.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list PRs %s/%s: %w", owner, repo, err)
		}

		for _, pr := range prs {
			if baseRef == "" {
				allPRs = append(allPRs, pr)
				continue
			}

			targetRef := pr.GetBase().GetRef()
			if targetRef == baseRef {
				allPRs = append(allPRs, pr)
			} else if matched, err := path.Match(baseRef, targetRef); err == nil && matched {
				allPRs = append(allPRs, pr)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allPRs, nil
}

// GetPR retrieves a pull request by number.
func (c *Client) GetPR(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
	pr, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("get PR %s/%s#%d: %w", owner, repo, number, err)
	}
	return pr, nil
}

// GetPRHeadSHA retrieves the head commit SHA for a pull request.
func (c *Client) GetPRHeadSHA(ctx context.Context, owner, repo string, number int) (string, error) {
	pr, err := c.GetPR(ctx, owner, repo, number)
	if err != nil {
		return "", err
	}
	sha := pr.GetHead().GetSHA()
	if sha == "" {
		return "", fmt.Errorf("PR %s/%s#%d has empty head SHA", owner, repo, number)
	}
	return sha, nil
}

// ListCheckRunsForSHA returns all check runs associated with a commit SHA or ref.
func (c *Client) ListCheckRunsForSHA(ctx context.Context, owner, repo, sha string) ([]*github.CheckRun, error) {
	var allRuns []*github.CheckRun
	opts := &github.ListCheckRunsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		result, resp, err := c.gh.Checks.ListCheckRunsForRef(ctx, owner, repo, sha, opts)
		if err != nil {
			return nil, fmt.Errorf("list check runs %s/%s@%s: %w", owner, repo, sha, err)
		}
		allRuns = append(allRuns, result.CheckRuns...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allRuns, nil
}

// GetCheckRun fetches a single check run by its ID.
func (c *Client) GetCheckRun(ctx context.Context, owner, repo string, checkRunID int64) (*github.CheckRun, error) {
	checkRun, _, err := c.gh.Checks.GetCheckRun(ctx, owner, repo, checkRunID)
	if err != nil {
		return nil, fmt.Errorf("get check run %s/%s#%d: %w", owner, repo, checkRunID, err)
	}
	return checkRun, nil
}

// FetchCheckRunOutput returns the output field of a check run by its ID.
func (c *Client) FetchCheckRunOutput(ctx context.Context, owner, repo string, checkRunID int64) (*github.CheckRunOutput, error) {
	checkRun, err := c.GetCheckRun(ctx, owner, repo, checkRunID)
	if err != nil {
		return nil, err
	}
	return checkRun.GetOutput(), nil
}

// AddLabels adds labels to an issue or pull request.
func (c *Client) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	_, _, err := c.gh.Issues.AddLabelsToIssue(ctx, owner, repo, number, labels)
	if err != nil {
		return fmt.Errorf("add labels %s/%s#%d: %w", owner, repo, number, err)
	}
	return nil
}

// CreateComment adds a comment to an issue or pull request and returns the created comment ID.
func (c *Client) CreateComment(ctx context.Context, owner, repo string, number int, body string) (int64, error) {
	comment, _, err := c.gh.Issues.CreateComment(ctx, owner, repo, number, &github.IssueComment{
		Body: github.Ptr(body),
	})
	if err != nil {
		return 0, fmt.Errorf("create comment %s/%s#%d: %w", owner, repo, number, err)
	}
	return comment.GetID(), nil
}
