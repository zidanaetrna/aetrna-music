# Contributing to aetrna-music

Thank you for your interest in contributing to **aetrna-music**! We welcome bug fixes, performance improvements, feature enhancements, and documentation updates.

---

## Clean Code Culture & Rules

To keep the repository clean, reliable, and enterprise-grade:

1. **Zero Emojis in Code, Logs & Documentation**:
   - Do not use informal unicode emojis in code comments, commit messages, documentation, or server log outputs.
   - Standardize all server logs to use enterprise bracketed log tags: `[INFO]`, `[WARN]`, `[ERROR]`, `[DEBUG]`.
   - Keep log messages clean, structured, and informative.
2. **Architecture Decoupling**:
   - The Go core service manages Discord bot commands, state, queue, and SQLite persistence.
   - The Node.js voice server handles opus audio encoding/decoding via `prism-media` & `dgram`.
   - Keep communication between services decoupled via IPC/WS.
3. **No Lavalink Dependencies**:
   - Do not re-introduce Lavalink or Java dependencies. The engine is strictly self-hosted native Go + Node.js.

---

## Local Development Setup

1. **Prerequisites**:
   - **Go**: `1.22+`
   - **Node.js**: `22+`
   - **FFmpeg**: Installed and available in your system `PATH`.
   - **yt-dlp**: Installed and available in your system `PATH`.

2. **Setup Instructions**:
   ```bash
   # Clone repository
   git clone https://github.com/zidanaetrna/aetrna-music.git
   cd aetrna-music

   # Install Node dependencies
   npm install

   # Copy environment file
   cp .env.example .env
   # Edit DISCORD_TOKEN in .env
   ```

3. **Running Services Locally**:
   - Terminal 1 (Voice Server): `node voice-server/server.js`
   - Terminal 2 (Go Core & Web Dashboard): `go run ./cmd/bot`

---

## Pull Request Workflow

1. Fork the repository and create your feature branch:
   ```bash
   git checkout -b feature/my-amazing-feature
   ```
2. Commit your changes with clear, descriptive commit messages:
   ```bash
   git commit -m "feat(voice): enhance opus buffer flow control"
   ```
3. Push to your branch and open a Pull Request against `main`.
4. Ensure code builds without errors (`go build ./...` and `node --check voice-server/server.js`).

Thank you for contributing to aetrna-music.
