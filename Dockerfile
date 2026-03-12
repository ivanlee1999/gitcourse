# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Stage 2: Build backend
FROM golang:1.22-alpine AS backend-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN go build -o server .

# Stage 3: Final image — nginx + Go backend
FROM alpine:3.19

# Install nginx and supervisor (to run both processes)
RUN apk add --no-cache nginx supervisor

# Copy Go binary
COPY --from=backend-builder /app/backend/server /app/server

# Copy frontend static files
COPY --from=frontend-builder /app/frontend/dist /usr/share/nginx/html

# nginx config: serve frontend + proxy /api to backend
RUN cat > /etc/nginx/http.d/default.conf << 'NGINX'
server {
    listen 80;

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location / {
        root /usr/share/nginx/html;
        try_files $uri $uri/ /index.html;
    }
}
NGINX

# supervisord config: run nginx + backend together
RUN cat > /etc/supervisord.conf << 'SUPERVISOR'
[supervisord]
nodaemon=true
logfile=/dev/null
logfile_maxbytes=0

[program:nginx]
command=nginx -g "daemon off;"
autostart=true
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0

[program:backend]
command=/app/server
autostart=true
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
SUPERVISOR

EXPOSE 80
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf"]
