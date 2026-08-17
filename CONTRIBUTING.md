# Contributing to aetrna-music

Thanks for wanting to contribute to **aetrna-music**!

Bug fixes, performance improvements, new features, documentation updates, random ideas that somehow turn into features - all welcome.

The project is still evolving, so if something looks weird, feel free to open an issue or pull request.

---

## Development Guidelines

### Keep It Clean

We're not trying to build enterprise software for a bank, but please don't make future us hate ourselves.

- Keep the code readable
- Use clear names
- Avoid unnecessary complexity
- Follow the existing style
- Keep comments useful
- Keep logs structured with `[INFO]`, `[WARN]`, `[ERROR]`, and `[DEBUG]`
- Avoid emojis in source code, logs, and technical documentation

The project can be casual. The code doesn't have to be cursed.

### Architecture

aetrna-music currently has two main parts:

- **Go Core** - Discord bot commands, queue/state management, Web API, authentication, and SQLite persistence.
- **Node.js Voice Worker** - Audio streaming, Discord voice handling, Opus processing, and the FFmpeg audio pipeline.

Try to keep their responsibilities separated.

If you need to change how they communicate, make sure the existing IPC/HTTP boundaries still make sense.

### No Lavalink

Please don't add Lavalink or Java dependencies.

The whole point is that aetrna-music is **Lavalink-free**.

We've already chosen the harder way.

---

## Local Development

### Prerequisites

- **Go:** `1.23+`
- **Node.js:** `22+`
- **FFmpeg:** Installed and available in `PATH`
- **yt-dlp:** Installed and available in `PATH`

### Setup

```bash
git clone https://github.com/zidanaetrna/aetrna-music.git
cd aetrna-music
cp .env.example .env
```

Edit `.env` and add your required configuration. The minimum you need to set is `DISCORD_TOKEN` and `ADMIN_KEY` (the legacy `DASHBOARD_PASSWORD` alias still works). In production also change `INTERNAL_IPC_TOKEN` from its dev-default.

**Install Node.js dependencies (Voice Worker):**
```bash
cd voice-server
npm install
cd ..
```

**Install Node.js dependencies (Web Dashboard, needed only if you change the React app):**
```bash
cd web
npm install
cd ..
```

**Build the Web Dashboard into `web/dist/` (required before any Go build):**
The Go binary embeds `web/dist` using `//go:embed all:dist`, so the compiled Vite output **must** exist before you `go run` or `go build`.
```bash
cd web
npm run build     # tsc (strict TypeScript check) + vite build
cd ..
```

**Do not commit `.env`, Discord tokens, YouTube cookies, passwords, or other secrets.**

### Run Locally

Terminal 1 — Node.js Voice Worker (listens on `VOICE_PORT`, default `:3005`; avoid the generic `PORT` env var to prevent colliding with the Go dashboard on `:8080`):

```bash
cd voice-server
node server.js
```

Terminal 2 — Go Core Bot + Web API (`:8080`). This also starts the internal IPC webhook listener on `:47392` for Node→Go interaction and track-end events (protected by `X-Internal-IPC-Token` header matching `INTERNAL_IPC_TOKEN`).

```bash
# web/dist must already exist (run `cd web && npm run build` first)
go run ./cmd/bot
```

If you are actively changing React/TS files and don't want to re-run `npm run build` constantly, start the Vite dev server instead and proxy `/api/*` to Go:
```bash
cd web
npm run dev       # http://localhost:5173, proxies /api → http://localhost:8080
```

Optional terminal 3 — tail the system logs through the public API once the bot is up:
```bash
# Log in first at http://localhost:8080 and copy the session token
curl -N "http://localhost:8080/api/logs?token=<session_token>"
```

---

## Making Changes

For small fixes, documentation changes, and obvious improvements, just open a pull request.

For larger features, opening an issue first is recommended so we can talk about it before someone spends three days implementing something nobody asked for.

When making changes:

- Keep the PR focused
- Avoid unrelated refactors
- Follow the existing architecture
- Test your changes
- Update the docs if necessary

---

## Pull Requests

### 1. Create a Branch

```bash
git checkout -b feature/my-feature
```

or:

```bash
git checkout -b fix/queue-playback-bug
```

### 2. Make Your Changes

Keep commits reasonably focused.

Example:

```bash
git commit -m "feat(voice): improve opus buffer flow control"
```

### 3. Run the Checks

**Go:**
```bash
go vet ./...
go build ./...
go test ./...
```

**Voice Server Node.js:**
```bash
node --check voice-server/server.js
```

**Web Dashboard TypeScript + Vite build:**
```bash
cd web
npm run build     # Strict `tsc` type-checking runs before vite build fails on any TS error
cd ..
```

**YouTube cookies file check:**
If you are testing with cookies, make sure `./cookies.txt` exists and exceeds **100 bytes**. Files below this threshold are silently skipped by both Go and Node to avoid yt-dlp `--cookies` errors.

Also test anything else affected by your changes.

### 4. Open a Pull Request

Open your PR against `main`.

Please include:

- What changed
- Why
- How you tested it
- Anything we should know

---

## Bug Reports

If you found something broken, please include:

- What happened
- What you expected
- How to reproduce it
- Relevant logs
- Go/Node.js versions
- Docker or manual deployment
- Anything else that might help

Please don't post your tokens, passwords, cookies, or other secrets.

---

## Feature Requests

Got an idea?

Go for it.

Just keep the project's main goals in mind:

- Self-hosted
- Lavalink-free
- Lightweight
- Easy to deploy
- Actually useful

Not every idea will make it in, but we're happy to hear them.

---

## One Last Thing

Don't be afraid to open a PR just because you're not sure if it's perfect.

We can figure it out.

Thanks for contributing to **aetrna-music**.

> Discord music bot. unfortunately.
