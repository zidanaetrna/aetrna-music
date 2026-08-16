# Multi-Stage Dockerfile for Aetrna's Music Platform

# Stage 1: Build Go Bot Microservice
FROM golang:1.22-alpine AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o aetrna-bot ./...

# Stage 2: Final Production Runtime Image
FROM node:22-alpine
WORKDIR /app

# Install system dependencies: FFmpeg, yt-dlp, Python3, CA certificates, tzdata
RUN apk add --no-cache \
    ffmpeg \
    yt-dlp \
    python3 \
    ca-certificates \
    tzdata \
    bash

# Copy Go binary from Stage 1
COPY --from=go-builder /build/aetrna-bot /app/aetrna-bot

# Copy Voice Server JS codebase & package files
COPY voice-server/package*.json /app/voice-server/
RUN cd /app/voice-server && npm ci --only=production

COPY voice-server/ /app/voice-server/
COPY bin/ /app/bin/
COPY package.json /app/
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh /app/aetrna-bot

# Create persistent data directory
RUN mkdir -p /app/data

EXPOSE 8080

ENTRYPOINT ["/app/entrypoint.sh"]
