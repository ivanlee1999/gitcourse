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

## Webhook Setup (recommended)

Configure a GitHub webhook for real-time updates instead of relying solely on polling:

1. Go to your GitHub org settings → Webhooks → Add webhook
2. **Payload URL:** `https://ci.tpcard.io/api/webhook/github`
3. **Content type:** `application/json`
4. **Secret:** value of your `GITHUB_WEBHOOK_SECRET` env var
5. **Events:** select "Workflow runs" and "Workflow jobs"

Set the `GITHUB_WEBHOOK_SECRET` env var on the server to enable signature verification.

## Features

- All workflows across an org in one view
- Sorted by most recent build
- Active builds banner with runner name + current step
- Workflow run history with job/step breakdown
- Real-time updates via GitHub webhooks + SSE
- Adaptive polling (60s default, backs off when rate limit is low)
- Only polls repos with workflows (skips archived repos)
- Rate limit indicator in dashboard
- Dark Concourse-inspired theme
