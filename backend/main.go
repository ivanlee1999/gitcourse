package main

import (
	"log"
	"net/http"
	"os"

	"github.com/ivanlee1999/gitcourse/backend/api"
	ghclient "github.com/ivanlee1999/gitcourse/backend/github"
)

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	org := os.Getenv("GITHUB_ORG")
	if org == "" {
		log.Fatal("GITHUB_ORG environment variable is required")
	}

	client := ghclient.NewClient(token)
	server := api.NewServer(client, org)

	// Pre-populate dashboard cache before accepting requests
	server.InitDashboardCache()
	server.StartBackgroundRefresh()

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on :%s for org %s", port, org)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
