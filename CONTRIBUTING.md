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
npm install
cp .env.example .env
```

Edit `.env` and add your required configuration, including your Discord bot token.

**Do not commit `.env`, Discord tokens, YouTube cookies, passwords, or other secrets.**

### Run Locally

Terminal 1:

```bash
node voice-server/server.js
```

Terminal 2:

```bash
go run ./cmd/bot
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

```bash
go build ./...
```

```bash
node --check voice-server/server.js
```

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
