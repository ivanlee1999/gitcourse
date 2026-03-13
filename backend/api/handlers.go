package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	ghclient "github.com/ivanlee1999/gitcourse/backend/github"
)

type sseClient struct {
	ch chan []byte
}

type Server struct {
	githubClient  *ghclient.Client
	org           string
	webhookSecret string

	// Background dashboard cache
	dashboardMu    sync.RWMutex
	dashboardCache []DashboardRepo

	// SSE clients
	sseMu      sync.Mutex
	sseClients map[*sseClient]struct{}

	// Repos with known workflows (only poll these)
	activeReposMu      sync.RWMutex
	activeRepos         map[string]bool // "owner/repo" -> true
	lastFullRepoScan    time.Time
	fullRepoScanInterval time.Duration

	// Rate limit state
	rateLimitMu   sync.RWMutex
	rateLimitInfo *ghclient.RateLimitInfo
}

func NewServer(ghClient *ghclient.Client, org, webhookSecret string) *Server {
	return &Server{
		githubClient:         ghClient,
		org:                  org,
		webhookSecret:        webhookSecret,
		sseClients:           make(map[*sseClient]struct{}),
		activeRepos:          make(map[string]bool),
		fullRepoScanInterval: 10 * time.Minute,
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard", corsMiddleware(http.HandlerFunc(s.handleDashboard)))
	mux.Handle("GET /api/events", corsMiddleware(http.HandlerFunc(s.handleSSE)))
	mux.Handle("GET /api/repos/{owner}/{repo}/workflows", corsMiddleware(http.HandlerFunc(s.handleWorkflows)))
	mux.Handle("GET /api/repos/{owner}/{repo}/workflows/{workflow_id}/runs", corsMiddleware(http.HandlerFunc(s.handleWorkflowRuns)))
	mux.Handle("GET /api/runs/{owner}/{repo}/{run_id}/jobs", corsMiddleware(http.HandlerFunc(s.handleJobs)))
	mux.Handle("POST /api/webhook/github", corsMiddleware(http.HandlerFunc(s.handleWebhook)))
	mux.Handle("GET /api/rate-limit", corsMiddleware(http.HandlerFunc(s.handleRateLimit)))
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
	ID        int64                 `json:"id"`
	Name      string                `json:"name"`
	State     string                `json:"state"`
	LatestRun *DashboardWorkflowRun `json:"latest_run"`
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
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
	HeadBranch   string    `json:"head_branch"`
	HeadSHA      string    `json:"head_sha"`
	Event        string    `json:"event"`
	RunNumber    int       `json:"run_number"`
	Actor        string    `json:"actor"`
	DisplayTitle string    `json:"display_title"`
	Jobs         []SlimJob `json:"jobs,omitempty"`
}

// discoverActiveRepos does a full scan of all org repos and identifies which ones have workflows.
// It skips archived repos entirely.
func (s *Server) discoverActiveRepos() error {
	ctx := context.Background()
	repos, err := s.githubClient.GetOrgRepos(ctx, s.org)
	if err != nil {
		return fmt.Errorf("failed to get repos: %w", err)
	}

	active := make(map[string]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for _, repo := range repos {
		if repo.GetArchived() {
			continue
		}
		wg.Add(1)
		go func(owner, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			workflows, err := s.githubClient.GetWorkflows(ctx, owner, name)
			if err != nil {
				log.Printf("error checking workflows for %s/%s: %v", owner, name, err)
				return
			}
			if workflows.TotalCount > 0 {
				mu.Lock()
				active[owner+"/"+name] = true
				mu.Unlock()
			}
		}(repo.GetOwner().GetLogin(), repo.GetName())
	}
	wg.Wait()

	s.activeReposMu.Lock()
	s.activeRepos = active
	s.lastFullRepoScan = time.Now()
	s.activeReposMu.Unlock()

	log.Printf("Discovered %d repos with workflows (out of %d total, excluding archived)", len(active), len(repos))
	return nil
}

// fetchDashboard fetches fresh dashboard data from GitHub, only for repos with known workflows.
func (s *Server) fetchDashboard() ([]DashboardRepo, error) {
	ctx := context.Background()

	// Re-scan all repos periodically to discover new workflows
	s.activeReposMu.RLock()
	needsFullScan := time.Since(s.lastFullRepoScan) >= s.fullRepoScanInterval
	s.activeReposMu.RUnlock()

	if needsFullScan {
		if err := s.discoverActiveRepos(); err != nil {
			log.Printf("Full repo scan failed: %v", err)
		}
	}

	s.activeReposMu.RLock()
	activeRepos := make(map[string]bool, len(s.activeRepos))
	for k, v := range s.activeRepos {
		activeRepos[k] = v
	}
	s.activeReposMu.RUnlock()

	type repoKey struct {
		owner, name string
	}
	var repoList []repoKey
	for key := range activeRepos {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) == 2 {
			repoList = append(repoList, repoKey{parts[0], parts[1]})
		}
	}

	results := make([]DashboardRepo, len(repoList))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for i, rk := range repoList {
		wg.Add(1)
		go func(idx int, owner, repoName string) {
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
		}(i, rk.owner, rk.name)
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

// fetchDashboardForRepo fetches fresh dashboard data for a single repo.
func (s *Server) fetchDashboardForRepo(owner, repoName string) (*DashboardRepo, error) {
	ctx := context.Background()

	dashRepo := &DashboardRepo{
		Repo:      repoName,
		Owner:     owner,
		Workflows: []DashboardWorkflow{},
	}

	workflows, err := s.githubClient.GetWorkflows(ctx, owner, repoName)
	if err != nil {
		return dashRepo, fmt.Errorf("error getting workflows for %s/%s: %w", owner, repoName, err)
	}

	latestRuns, err := s.githubClient.GetLatestWorkflowRuns(ctx, owner, repoName)
	if err != nil {
		log.Printf("error getting latest runs for %s/%s: %v", owner, repoName, err)
	}

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

	for wfID, lr := range latestRunMap {
		if lr.Status == "in_progress" || lr.Status == "queued" {
			jobs, err := s.githubClient.GetJobsForRun(ctx, owner, repoName, lr.ID)
			if err != nil {
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

	return dashRepo, nil
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
	log.Println("Discovering repos with workflows...")
	if err := s.discoverActiveRepos(); err != nil {
		log.Printf("Warning: initial repo discovery failed: %v", err)
	}

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

// refreshInterval returns the current refresh interval based on rate limit status.
func (s *Server) refreshInterval() time.Duration {
	s.rateLimitMu.RLock()
	rl := s.rateLimitInfo
	s.rateLimitMu.RUnlock()

	if rl != nil {
		if rl.Remaining < 100 {
			return 0 // pause entirely
		}
		if rl.Remaining < 500 {
			return 5 * time.Minute
		}
	}
	return 60 * time.Second
}

// StartBackgroundRefresh starts a goroutine that refreshes the dashboard cache adaptively.
func (s *Server) StartBackgroundRefresh() {
	go func() {
		for {
			interval := s.refreshInterval()
			if interval == 0 {
				log.Println("Rate limit critically low, pausing background refresh for 60s")
				time.Sleep(60 * time.Second)
				// Re-check rate limit
				s.updateRateLimit()
				continue
			}

			time.Sleep(interval)

			// Check rate limit before refresh (free call)
			s.updateRateLimit()
			if ri := s.refreshInterval(); ri == 0 {
				continue
			}

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

// updateRateLimit fetches the current rate limit and stores it.
func (s *Server) updateRateLimit() {
	ctx := context.Background()
	rl, err := s.githubClient.GetRateLimit(ctx)
	if err != nil {
		log.Printf("Failed to check rate limit: %v", err)
		return
	}
	s.rateLimitMu.Lock()
	s.rateLimitInfo = rl
	s.rateLimitMu.Unlock()
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

// handleWebhook processes GitHub webhook events for workflow_run and workflow_job.
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	// Verify signature if webhook secret is configured
	if s.webhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if sig == "" {
			writeError(w, http.StatusUnauthorized, "missing signature")
			return
		}
		if !verifySignature(body, sig, s.webhookSecret) {
			writeError(w, http.StatusUnauthorized, "invalid signature")
			return
		}
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType != "workflow_run" && eventType != "workflow_job" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Extract repo info from the payload
	var payload struct {
		Repository struct {
			Name  string `json:"name"`
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	owner := payload.Repository.Owner.Login
	repoName := payload.Repository.Name

	// Mark this repo as active
	repoKey := owner + "/" + repoName
	s.activeReposMu.Lock()
	s.activeRepos[repoKey] = true
	s.activeReposMu.Unlock()

	// Invalidate cache for this repo and re-fetch
	s.githubClient.InvalidateRepo(owner, repoName)

	go func() {
		updated, err := s.fetchDashboardForRepo(owner, repoName)
		if err != nil {
			log.Printf("Webhook: failed to refresh %s/%s: %v", owner, repoName, err)
			return
		}

		s.dashboardMu.Lock()
		found := false
		for i, dr := range s.dashboardCache {
			if dr.Owner == owner && dr.Repo == repoName {
				s.dashboardCache[i] = *updated
				found = true
				break
			}
		}
		if !found {
			s.dashboardCache = append(s.dashboardCache, *updated)
		}
		data := make([]DashboardRepo, len(s.dashboardCache))
		copy(data, s.dashboardCache)
		s.dashboardMu.Unlock()

		// Re-sort
		sort.Slice(data, func(i, j int) bool {
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
			return latestTime(data[i]).After(latestTime(data[j]))
		})

		s.dashboardMu.Lock()
		s.dashboardCache = data
		s.dashboardMu.Unlock()

		s.broadcastSSE(data)
		log.Printf("Webhook: updated dashboard for %s/%s (%s)", owner, repoName, eventType)
	}()

	w.WriteHeader(http.StatusOK)
}

// verifySignature validates the GitHub webhook HMAC-SHA256 signature.
func verifySignature(payload []byte, signature, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	sig, err := hex.DecodeString(signature[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := mac.Sum(nil)
	return hmac.Equal(sig, expected)
}

// handleRateLimit returns current GitHub API rate limit info.
func (s *Server) handleRateLimit(w http.ResponseWriter, r *http.Request) {
	s.rateLimitMu.RLock()
	rl := s.rateLimitInfo
	s.rateLimitMu.RUnlock()

	if rl == nil {
		// Fetch it now
		s.updateRateLimit()
		s.rateLimitMu.RLock()
		rl = s.rateLimitInfo
		s.rateLimitMu.RUnlock()
	}

	if rl == nil {
		writeError(w, http.StatusServiceUnavailable, "rate limit info unavailable")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"limit":     rl.Limit,
		"remaining": rl.Remaining,
		"reset":     rl.Reset.Format(time.RFC3339),
	})
}
