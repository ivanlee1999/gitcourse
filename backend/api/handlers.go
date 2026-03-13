package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	ghclient "github.com/ivanlee1999/gitcourse/backend/github"
)

type sseClient struct {
	ch chan []byte
}

type Server struct {
	githubClient *ghclient.Client
	org          string

	// Background dashboard cache
	dashboardMu    sync.RWMutex
	dashboardCache []DashboardRepo

	// SSE clients
	sseMu      sync.Mutex
	sseClients map[*sseClient]struct{}
}

func NewServer(ghClient *ghclient.Client, org string) *Server {
	return &Server{
		githubClient: ghClient,
		org:          org,
		sseClients:   make(map[*sseClient]struct{}),
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard", corsMiddleware(http.HandlerFunc(s.handleDashboard)))
	mux.Handle("GET /api/events", corsMiddleware(http.HandlerFunc(s.handleSSE)))
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
	ID          int64  `json:"id"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	HeadBranch  string `json:"head_branch"`
	Event       string `json:"event"`
	Actor       string `json:"actor"`
	RunNumber   int    `json:"run_number"`
	RunnerName  string `json:"runner_name,omitempty"`
	CurrentStep string `json:"current_step,omitempty"`
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


type SlimJob struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	RunnerName  string `json:"runner_name"`
	StartedAt   string `json:"started_at"`
	CurrentStep string `json:"current_step"`
}

type SlimWorkflowRun struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	Conclusion   string     `json:"conclusion"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
	HeadBranch   string     `json:"head_branch"`
	HeadSHA      string     `json:"head_sha"`
	Event        string     `json:"event"`
	RunNumber    int        `json:"run_number"`
	Actor        string     `json:"actor"`
	DisplayTitle string     `json:"display_title"`
	Jobs         []SlimJob  `json:"jobs,omitempty"`
}

// fetchDashboard fetches fresh dashboard data from GitHub.
func (s *Server) fetchDashboard() ([]DashboardRepo, error) {
	ctx := context.Background()

	repos, err := s.githubClient.GetOrgRepos(ctx, s.org)
	if err != nil {
		return nil, fmt.Errorf("failed to get repos: %w", err)
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

			// For in_progress runs, fetch jobs to get runner_name and current_step
			for wfID, lr := range latestRunMap {
				if lr.Status == "in_progress" || lr.Status == "queued" {
					jobs, err := s.githubClient.GetJobsForRun(ctx, owner, repoName, lr.ID)
					if err != nil {
						log.Printf("error getting jobs for run %d: %v", lr.ID, err)
						continue
					}
					for _, job := range jobs {
						if job.GetStatus() == "in_progress" {
							lr.RunnerName = job.GetRunnerName()
							for _, step := range job.Steps {
								if step.GetStatus() == "in_progress" {
									lr.CurrentStep = step.GetName()
									break
								}
							}
							break
						}
					}
					latestRunMap[wfID] = lr
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

	// Sort repos by most recent build time (repos with no runs go to bottom)
	sort.Slice(results, func(i, j int) bool {
		latestTime := func(r DashboardRepo) time.Time {
			var t time.Time
			for _, wf := range r.Workflows {
				if wf.LatestRun != nil {
					parsed, err := time.Parse("2006-01-02T15:04:05Z", wf.LatestRun.UpdatedAt)
					if err == nil && parsed.After(t) {
						t = parsed
					}
				}
			}
			return t
		}
		return latestTime(results[i]).After(latestTime(results[j]))
	})

	return results, nil
}

// dashboardChanged compares old and new dashboard data for status/conclusion changes.
func dashboardChanged(old, new []DashboardRepo) bool {
	if len(old) != len(new) {
		return true
	}
	oldRuns := make(map[int64][2]string) // run ID -> [status, conclusion]
	for _, repo := range old {
		for _, wf := range repo.Workflows {
			if wf.LatestRun != nil {
				oldRuns[wf.LatestRun.ID] = [2]string{wf.LatestRun.Status, wf.LatestRun.Conclusion}
			}
		}
	}
	for _, repo := range new {
		for _, wf := range repo.Workflows {
			if wf.LatestRun != nil {
				prev, exists := oldRuns[wf.LatestRun.ID]
				if !exists || prev[0] != wf.LatestRun.Status || prev[1] != wf.LatestRun.Conclusion {
					return true
				}
			}
		}
	}
	return false
}

// InitDashboardCache pre-populates the dashboard cache. Call before starting the server.
func (s *Server) InitDashboardCache() {
	log.Println("Pre-populating dashboard cache...")
	data, err := s.fetchDashboard()
	if err != nil {
		log.Printf("Warning: initial dashboard fetch failed: %v", err)
		return
	}
	s.dashboardMu.Lock()
	s.dashboardCache = data
	s.dashboardMu.Unlock()
	log.Printf("Dashboard cache populated with %d repos", len(data))
}

// StartBackgroundRefresh starts a goroutine that refreshes the dashboard cache every 30s.
func (s *Server) StartBackgroundRefresh() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			data, err := s.fetchDashboard()
			if err != nil {
				log.Printf("Background refresh failed: %v", err)
				continue
			}

			s.dashboardMu.RLock()
			changed := dashboardChanged(s.dashboardCache, data)
			s.dashboardMu.RUnlock()

			s.dashboardMu.Lock()
			s.dashboardCache = data
			s.dashboardMu.Unlock()

			if changed {
				s.broadcastSSE(data)
			}
		}
	}()
}

// broadcastSSE sends dashboard data to all connected SSE clients.
func (s *Server) broadcastSSE(data []DashboardRepo) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("SSE marshal error: %v", err)
		return
	}

	s.sseMu.Lock()
	defer s.sseMu.Unlock()
	for client := range s.sseClients {
		select {
		case client.ch <- payload:
		default:
			// Client too slow, skip this event
		}
	}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	s.dashboardMu.RLock()
	data := s.dashboardCache
	s.dashboardMu.RUnlock()

	if data == nil {
		data = []DashboardRepo{}
	}
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	client := &sseClient{ch: make(chan []byte, 16)}

	s.sseMu.Lock()
	s.sseClients[client] = struct{}{}
	s.sseMu.Unlock()

	defer func() {
		s.sseMu.Lock()
		delete(s.sseClients, client)
		s.sseMu.Unlock()
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-client.ch:
			fmt.Fprintf(w, "event: dashboard\ndata: %s\n\n", payload)
			flusher.Flush()
		}
	}
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

	ctx := r.Context()

	slim := make([]SlimWorkflowRun, 0, len(runs))
	for _, run := range runs {
		sr := SlimWorkflowRun{
			ID:           run.GetID(),
			Name:         run.GetName(),
			Status:       run.GetStatus(),
			Conclusion:   run.GetConclusion(),
			CreatedAt:    run.GetCreatedAt().Format("2006-01-02T15:04:05Z"),
			UpdatedAt:    run.GetUpdatedAt().Format("2006-01-02T15:04:05Z"),
			HeadBranch:   run.GetHeadBranch(),
			HeadSHA:      run.GetHeadSHA(),
			Event:        run.GetEvent(),
			RunNumber:    run.GetRunNumber(),
			Actor:        run.GetActor().GetLogin(),
			DisplayTitle: run.GetDisplayTitle(),
		}

		if run.GetStatus() == "in_progress" || run.GetStatus() == "queued" {
			jobs, err := s.githubClient.GetJobsForRun(ctx, owner, repo, run.GetID())
			if err == nil {
				for _, job := range jobs {
					currentStep := ""
					for _, step := range job.Steps {
						if step.GetStatus() == "in_progress" {
							currentStep = step.GetName()
							break
						}
					}
					sr.Jobs = append(sr.Jobs, SlimJob{
						ID:          job.GetID(),
						Name:        job.GetName(),
						Status:      job.GetStatus(),
						Conclusion:  job.GetConclusion(),
						RunnerName:  job.GetRunnerName(),
						StartedAt:   job.GetStartedAt().Format("2006-01-02T15:04:05Z"),
						CurrentStep: currentStep,
					})
				}
			}
		}

		slim = append(slim, sr)
	}
	writeJSON(w, http.StatusOK, slim)
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
