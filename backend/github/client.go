package github

import (
	"context"
	"fmt"
	"time"

	gh "github.com/google/go-github/v60/github"
	"github.com/ivanlee1999/gitcourse/backend/cache"
	"golang.org/x/oauth2"
)

const cacheTTL = 30 * time.Second

type Client struct {
	gh    *gh.Client
	cache *cache.Cache
}

func NewClient(token string) *Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	client := gh.NewClient(tc)

	return &Client{
		gh:    client,
		cache: cache.New(),
	}
}

func (c *Client) GetOrgRepos(ctx context.Context, org string) ([]*gh.Repository, error) {
	cacheKey := fmt.Sprintf("repos:%s", org)
	if data, ok := c.cache.Get(cacheKey); ok {
		return data.([]*gh.Repository), nil
	}

	var allRepos []*gh.Repository
	opts := &gh.RepositoryListByOrgOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	for {
		repos, resp, err := c.gh.Repositories.ListByOrg(ctx, org, opts)
		if err != nil {
			return nil, fmt.Errorf("listing repos for org %s: %w", org, err)
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	c.cache.Set(cacheKey, allRepos, cacheTTL)
	return allRepos, nil
}

type WorkflowsResult struct {
	TotalCount int
	Workflows  []*gh.Workflow
}

func (c *Client) GetWorkflows(ctx context.Context, owner, repo string) (*WorkflowsResult, error) {
	cacheKey := fmt.Sprintf("workflows:%s/%s", owner, repo)
	if data, ok := c.cache.Get(cacheKey); ok {
		return data.(*WorkflowsResult), nil
	}

	workflows, _, err := c.gh.Actions.ListWorkflows(ctx, owner, repo, nil)
	if err != nil {
		return nil, fmt.Errorf("listing workflows for %s/%s: %w", owner, repo, err)
	}

	result := &WorkflowsResult{
		TotalCount: workflows.GetTotalCount(),
		Workflows:  workflows.Workflows,
	}

	c.cache.Set(cacheKey, result, cacheTTL)
	return result, nil
}

func (c *Client) GetWorkflowRuns(ctx context.Context, owner, repo string, workflowID int64) ([]*gh.WorkflowRun, error) {
	cacheKey := fmt.Sprintf("runs:%s/%s/%d", owner, repo, workflowID)
	if data, ok := c.cache.Get(cacheKey); ok {
		return data.([]*gh.WorkflowRun), nil
	}

	opts := &gh.ListWorkflowRunsOptions{
		ListOptions: gh.ListOptions{PerPage: 20},
	}
	runs, _, err := c.gh.Actions.ListWorkflowRunsByID(ctx, owner, repo, workflowID, opts)
	if err != nil {
		return nil, fmt.Errorf("listing runs for workflow %d in %s/%s: %w", workflowID, owner, repo, err)
	}

	c.cache.Set(cacheKey, runs.WorkflowRuns, cacheTTL)
	return runs.WorkflowRuns, nil
}

func (c *Client) GetLatestWorkflowRuns(ctx context.Context, owner, repo string) ([]*gh.WorkflowRun, error) {
	cacheKey := fmt.Sprintf("latest-runs:%s/%s", owner, repo)
	if data, ok := c.cache.Get(cacheKey); ok {
		return data.([]*gh.WorkflowRun), nil
	}

	opts := &gh.ListWorkflowRunsOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	}
	runs, _, err := c.gh.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("listing latest runs for %s/%s: %w", owner, repo, err)
	}

	c.cache.Set(cacheKey, runs.WorkflowRuns, cacheTTL)
	return runs.WorkflowRuns, nil
}

func (c *Client) GetJobsForRun(ctx context.Context, owner, repo string, runID int64) ([]*gh.WorkflowJob, error) {
	cacheKey := fmt.Sprintf("jobs:%s/%s/%d", owner, repo, runID)
	if data, ok := c.cache.Get(cacheKey); ok {
		return data.([]*gh.WorkflowJob), nil
	}

	jobs, _, err := c.gh.Actions.ListWorkflowJobs(ctx, owner, repo, runID, nil)
	if err != nil {
		return nil, fmt.Errorf("listing jobs for run %d in %s/%s: %w", runID, owner, repo, err)
	}

	c.cache.Set(cacheKey, jobs.Jobs, cacheTTL)
	return jobs.Jobs, nil
}
