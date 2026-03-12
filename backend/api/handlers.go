package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"

	ghclient "github.com/ivanlee1999/gitcourse/backend/github"
)

type Server struct {
	githubClient *ghclient.Client
	org          string
}

func NewServer(ghClient *ghclient.Client, org string) *Server {
	return &Server{
		githubClient: ghClient,
		org:          org,
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard", corsMiddleware(http.HandlerFunc(s.handleDashboard)))
	mux.Handle("GET /api/repos/{owner}/{repo}/workflows", corsMiddleware(http.HandlerFunc(s.handleWorkflows)))
	mux.Handle("GET /api/repos/{owner}/{repo}/workflows/{workflow_id}/runs", corsMiddleware(http.HandlerFunc(s.handleWorkflowRuns)))
	mux.Handle("GET /api/runs/{owner}/{repo}/{run_id}/jobs", corsMiddleware(http.HandlerFunc(s.handleJobs)))
	// Handle OPTIONS preflight for all api routes
	mux.Handle("OPTIONS /api/", corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("error encoding JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Dashboard types

type DashboardWorkflowRun struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	HeadBranch string `json:"head_branch"`
	Event      string `json:"event"`
	Actor      string `json:"actor"`
	RunNumber  int    `json:"run_number"`
}

type DashboardWorkflow struct {
	ID        int64                  `json:"id"`
	Name      string                 `json:"name"`
	State     string                 `json:"state"`
	LatestRun *DashboardWorkflowRun  `json:"latest_run"`
}

type DashboardRepo struct {
	Repo      string              `json:"repo"`
	Owner     string              `json:"owner"`
	Workflows []DashboardWorkflow `json:"workflows"`
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	repos, err := s.githubClient.GetOrgRepos(ctx, s.org)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get repos: %v", err))
		return
	}

	results := make([]DashboardRepo, len(repos))
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Bounded concurrency with semaphore of 10
	sem := make(chan struct{}, 10)

	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, repoName, owner string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			dashRepo := DashboardRepo{
				Repo:      repoName,
				Owner:     owner,
				Workflows: []DashboardWorkflow{},
			}

			workflows, err := s.githubClient.GetWorkflows(ctx, owner, repoName)
			if err != nil {
				log.Printf("error getting workflows for %s/%s: %v", owner, repoName, err)
				mu.Lock()
				results[idx] = dashRepo
				mu.Unlock()
				return
			}

			latestRuns, err := s.githubClient.GetLatestWorkflowRuns(ctx, owner, repoName)
			if err != nil {
				log.Printf("error getting latest runs for %s/%s: %v", owner, repoName, err)
			}

			// Build a map of workflow_id -> latest run
			latestRunMap := make(map[int64]*DashboardWorkflowRun)
			if latestRuns != nil {
				for _, run := range latestRuns {
					wfID := run.GetWorkflowID()
					if _, exists := latestRunMap[wfID]; !exists {
						actor := ""
						if run.GetActor() != nil {
							actor = run.GetActor().GetLogin()
						}
						latestRunMap[wfID] = &DashboardWorkflowRun{
							ID:         run.GetID(),
							Status:     run.GetStatus(),
							Conclusion: run.GetConclusion(),
							CreatedAt:  run.GetCreatedAt().Format("2006-01-02T15:04:05Z"),
							UpdatedAt:  run.GetUpdatedAt().Format("2006-01-02T15:04:05Z"),
							HeadBranch: run.GetHeadBranch(),
							Event:      run.GetEvent(),
							Actor:      actor,
							RunNumber:  run.GetRunNumber(),
						}
					}
				}
			}

			for _, wf := range workflows.Workflows {
				dw := DashboardWorkflow{
					ID:    wf.GetID(),
					Name:  wf.GetName(),
					State: wf.GetState(),
				}
				if lr, ok := latestRunMap[wf.GetID()]; ok {
					dw.LatestRun = lr
				}
				dashRepo.Workflows = append(dashRepo.Workflows, dw)
			}

			mu.Lock()
			results[idx] = dashRepo
			mu.Unlock()
		}(i, repo.GetName(), repo.GetOwner().GetLogin())
	}

	wg.Wait()
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleWorkflows(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")

	workflows, err := s.githubClient.GetWorkflows(r.Context(), owner, repo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get workflows: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, workflows)
}

func (s *Server) handleWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	wfIDStr := r.PathValue("workflow_id")

	workflowID, err := strconv.ParseInt(wfIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workflow_id")
		return
	}

	runs, err := s.githubClient.GetWorkflowRuns(r.Context(), owner, repo, workflowID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get workflow runs: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	runIDStr := r.PathValue("run_id")

	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run_id")
		return
	}

	jobs, err := s.githubClient.GetJobsForRun(r.Context(), owner, repo, runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get jobs: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, jobs)
}
