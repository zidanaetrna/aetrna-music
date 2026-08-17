# Multi-Stage Dockerfile for Aetrna's Music Platform
# ──────────────────────────────────────────────────────────────────────────────
# Stage 0 : Web Dashboard Builder (React + TypeScript + Vite)
#           Compiles web/ source code into web/dist/ static assets, which are
#           later embedded into the Go binary via //go:embed all:dist.
# ──────────────────────────────────────────────────────────────────────────────
FROM node:22-alpine AS web-builder
WORKDIR /build/web

# Install deps first (better layer caching — only re-run if package files change)
COPY web/package*.json ./
RUN npm install --no-audit --no-fund

# Copy the ENTIRE web/ source tree (tsconfig, vite config, index.html, src/,
# public/, assets/, types/, context/, hooks/, test/, etc.). Vite will throw
# only what it actually uses into the bundle; extra test files don't bloat the
# final web/dist/ output and we get 100% deterministic source coverage.
COPY web/ ./

# Build the dashboard. Output goes to web/dist/ which is embedded later.
RUN npm run build

# ──────────────────────────────────────────────────────────────────────────────
# Stage 1 : Go Bot Binary Builder
#           Compiles the Go bot, embedding web/dist/ (from Stage 0) into the
#           final binary so the dashboard UI ships as a single-file deploy.
# ──────────────────────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS go-builder
WORKDIR /build

# Go modules layer (cached independently)
COPY go.mod go.sum ./
RUN go mod download

# Copy all Go/infra source, then OVERLAY web/dist/ with the real build output
# from the web-builder stage. This replaces the .gitkeep placeholder that
# lives in git with the actual compiled React dashboard artifacts.
COPY . .
RUN rm -rf /build/web/dist
COPY --from=web-builder /build/web/dist /build/web/dist

# Build the statically-linked Go bot binary. web.FS embed directive now picks
# up the compiled dashboard artifacts.
RUN CGO_ENABLED=0 GOOS=linux go build -o aetrna-bot ./cmd/bot

# ──────────────────────────────────────────────────────────────────────────────
# Stage 2 : Final Production Runtime Image
#           Node.js (for voice server) + FFmpeg/yt-dlp + Go binary.
# ──────────────────────────────────────────────────────────────────────────────
FROM node:22-alpine
WORKDIR /app

# System-level deps: FFmpeg (audio decode/encode), yt-dlp (stream source),
# Python3/C++ toolchain required by sodium-native / opusscript native addons.
RUN apk add --no-cache \
    ffmpeg \
    yt-dlp \
    python3 \
    make \
    g++ \
    build-base \
    ca-certificates \
    tzdata \
    bash

# Copy the compiled Go bot from Stage 1 (has dashboard UI already embedded)
COPY --from=go-builder /build/aetrna-bot /app/aetrna-bot

# Voice server: install deps then copy JS sources
COPY voice-server/package*.json /app/voice-server/
RUN cd /app/voice-server && npm install --legacy-peer-deps --omit=dev --no-audit --no-fund

COPY voice-server/ /app/voice-server/
COPY bin/ /app/bin/
COPY package.json /app/
COPY entrypoint.sh /app/entrypoint.sh
# Normalise line endings (LF) so entrypoint.sh doesn't get
# "/usr/bin/env: 'bash\r': No such file or directory" on Linux containers,
# then ensure both executables have the +x bit.
RUN sed -i 's/\r$//' /app/entrypoint.sh && chmod +x /app/entrypoint.sh /app/aetrna-bot

# Persistent storage for SQLite DB, cookies.txt, yt-dlp cache, etc.
RUN mkdir -p /app/data

EXPOSE 8080

ENTRYPOINT ["/app/entrypoint.sh"]
