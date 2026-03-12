# GitHub Actions Dashboard

A Concourse-style dashboard for monitoring GitHub Actions across all repos in an org.

## Run with Docker (single image)

```bash
docker run -p 3000:80 \
  -e GITHUB_TOKEN=ghp_xxx \
  -e GITHUB_ORG=your-org \
  ivanlee1999/gh-actions-dashboard:latest
```

Open http://localhost:3000

## Run with Docker Compose

```bash
cp .env.example .env
# Edit .env with your GITHUB_TOKEN and GITHUB_ORG
docker-compose up
```

## Run locally (dev)

```bash
# Backend
cd backend
GITHUB_TOKEN=ghp_xxx GITHUB_ORG=your-org PORT=8090 go run main.go

# Frontend (separate terminal)
cd frontend
npm install
npm run dev
```

## Required GitHub Token Scopes

- `repo` — list repositories
- `read:org` — list org repos
- `workflow` — read Actions runs

## Features

- All workflows across an org in one view
- Sorted by most recent build
- Active builds banner with runner name + current step
- Workflow run history with job/step breakdown
- Auto-refresh (30s dashboard, 10s detail)
- Dark Concourse-inspired theme
