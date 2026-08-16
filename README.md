<div align="center">

<img src="web/public/artwork.png" alt="aetrna-music Official Artwork" width="120" height="120" style="border-radius: 50%; object-fit: cover; border: 3px solid #10B981; box-shadow: 0 0 20px rgba(16, 185, 129, 0.4);" /><br />
<sub><i>Official Bot Artwork by <b>@br_lie</b></i></sub>

# aetrna-music

**A high-performance, Lavalink-free, native Discord Music Engine & Web Dashboard**

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![NodeJS](https://img.shields.io/badge/Node.js-22+-339933?style=for-the-badge&logo=nodedotjs&logoColor=white)](https://nodejs.org/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=for-the-badge&logo=docker&logoColor=white)](https://www.docker.com/)
[![Web Dashboard](https://img.shields.io/badge/Web_Dashboard-v2.1.0_React_TS-10B981?style=for-the-badge&logo=react&logoColor=white)](#web-control-panel--dashboard)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg?style=for-the-badge)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-Welcome-brightgreen.svg?style=for-the-badge)](CONTRIBUTING.md)

<br /><br />

> *“Gua cuma Professional AI Prompter. Kalo kodenya agak ajaib tapi lagunya muter lancar jaya, berarti prompt gua gacor.”*
> <br />
> — **[zidanaetrna](https://github.com/zidanaetrna)** *(Professional AI Prompter)*

---

</div>

## Features Highlights

- **100% Lavalink-Free Architecture**: No Java runtime or bulky Lavalink servers required. Consumes only **15–30 MB RAM** per instance using a lightweight native Go core + Node.js voice worker.
- **Enterprise React 18 + TypeScript Web Dashboard**: Self-hosted split-screen SPA built with React 18, TypeScript, Vite, Cloudflare dark charcoal UI (`#0D0E12`), multi-guild target selector, real-time WebSocket telemetry engine (`/api/ws`), toast notifications, and multi-language support (EN, ID, JP).
- **Interactive Discord UI Cards**:
  - Full-width dynamic album cover cards.
  - Interactive Now Playing Control Bar (Pause, Skip, Prev, Loop, Shuffle, Vol+, Vol-, Filter, Favorite, Stop).
  - Search dropdown select menus.
  - Paginated queue tables with visual playback progress bars.
- **SQLite Persistence Engine**: WAL-mode SQLite database for instant user song favorites (`/favorite`) and custom user playlists (`/collection`).
- **Dynamic Audio DSP Filters**: On-the-fly FFmpeg equalizer filters (Bassboost, Nightcore, Vaporwave, 8D Audio, Pop).
- **Synchronized LRC Lyrics Engine (`/lyrics`)**: Real-time LRCLIB API integration fetching synced line-by-line LRC lyrics directly in Discord embeds.
- **Single Container Deployment**: Multi-stage POSIX supervisor container bundling Go, Node.js, FFmpeg, and yt-dlp into one lightweight container.
- **Interactive CLI Setup Wizard**: Initialize configuration in seconds with `npx aetrna-music init`.

---

## Architecture Overview

```mermaid
graph TD
    subgraph Discord Client & Web Users
        A[Discord User / Slash Commands]
        W[Web Dashboard Browser http://localhost:8080]
    end

    subgraph aetrna-music Single Container / Server
        subgraph Go Core Engine
            B[Go Bot Server]
            C[SQLite Database WAL]
            D[Web API & Embedded Static Server]
            E[Auth Manager HMAC-SHA256]
        end

        subgraph Node.js Voice Server
            F[Voice Worker Server port 3005]
            G[yt-dlp Audio Stream Engine]
            H[FFmpeg DSP Audio Filter Pipeline]
            I[Prism-Media Opus Encoder]
        end
    end

    subgraph External Gateways
        J[YouTube / Audio Sources]
        K[Discord Voice Gateway UDP/WS]
    end

    A -->|Slash / Legacy Cmds| B
    W -->|HTTP REST / Auth| D
    D --> E
    B -->|State & Favorites| C
    B -->|Local IPC / HTTP| F
    F -->|Fetch Track| G
    G -->|Stream Raw Audio| J
    G --> H
    H --> I
    I -->|Opus RTP Streams| K
```

---

## Quick Start & Installation

Choose your preferred deployment method:

### Option 1: Interactive CLI Setup Wizard (Recommended)

Run the zero-friction interactive wizard to generate `.env` and initialize configuration:

```bash
npx aetrna-music init
```

### Option 2: Docker Compose (1-Click Container)

1. **Clone repository**:
   ```bash
   git clone https://github.com/zidanaetrna/aetrna-music.git
   cd aetrna-music
   ```

2. **Copy environment template**:
   ```bash
   cp .env.example .env
   ```
   Edit `.env` and set your `DISCORD_TOKEN` and `DASHBOARD_PASSWORD`.

3. **Start container**:
   ```bash
   docker compose up -d
   ```
   *Your bot is now running, and the Web Dashboard is live at `http://localhost:8080`!*

### Option 3: Manual Execution (Local Run)

#### Prerequisites:
- **Go**: `1.23+`
- **Node.js**: `22+`
- **FFmpeg**: Installed and added to system `PATH`
- **yt-dlp**: Installed and added to system `PATH`

1. **Install Node dependencies**:
   ```bash
   npm install
   ```

2. **Start Voice Worker (Terminal 1)**:
   ```bash
   node voice-server/server.js
   ```

3. **Start Go Core Bot & Web Server (Terminal 2)**:
   ```bash
   go run ./cmd/bot
   ```

---

## YouTube Cookies Configuration

YouTube frequently blocks data center IPs or restricts age-gated tracks. To ensure smooth, uninterrupted audio playback, you can supply a `youtube_cookies.txt` file.

For step-by-step instructions on exporting cookies using browser extensions, refer to the **[YouTube Cookies Setup Guide](docs/YOUTUBE_COOKIES.md)**.

---

## Web Control Panel / Dashboard

`aetrna-music` comes with a modern React 18 + TypeScript Web Dashboard accessible at `http://localhost:8080`.

- **React 18 + TypeScript + Vite**: Built with modern component architecture, strict type safety, and zero external JS runtime overhead (`//go:embed all:dist`).
- **Real-Time Telemetry Stream**: WebSocket engine (`/api/ws`) broadcasting sub-millisecond RAM telemetry, active guild counts, and server uptime across browser tabs.
- **Cloudflare Dark Charcoal UI**: Collapsible sidebar (`250px` vs `64px`), quick search modal shortcut (`Ctrl + K`), and unified Deep Emerald Green theme (`#10B981`).
- **Multi-Guild Target Selector**: Switch between connected Discord servers to manage active queues, voice disconnects/kicks, and playback controls per guild.
- **Dynamic Multi-Language & Toasts**: Real-time language switching (English, Natural Tech Indonesian, Japanese) with sleek toast notifications.
- **HMAC-SHA256 Password Authentication**: Session protection configured via `.env` (`DASHBOARD_PASSWORD`).
- **REST API & WebSocket Endpoints**:
  - `POST /api/login` — Authenticate and receive HMAC session token.
  - `GET /api/status` — Get bot stats and active guild queues.
  - `POST /api/control` — Send playback control commands.
  - `WS /api/ws` — Real-time telemetry WebSocket stream.

---

## Command Reference

| Command | Category | Description |
|---|---|---|
| `/play <query>` | Playback | Search or queue track/playlist from YouTube or Spotify |
| `/search <query>` | Playback | Search tracks with interactive dropdown menu |
| `/pause` / `/resume` | Playback | Pause or resume audio playback |
| `/skip` | Playback | Skip to the next track in queue |
| `/stop` | Playback | Stop playback, clear queue, and leave voice channel |
| `/queue` | Queue | View paginated queue table with progress bar |
| `/nowplaying` | Queue | View Now Playing card with interactive control buttons |
| `/lyrics` | Playback | Fetch synced line-by-line LRC lyrics for current track |
| `/favorite` | Favorites | Save current song to your personal SQLite favorites |
| `/favorites` | Favorites | View and play your saved favorite songs |
| `/collection` | Collection | Save or load custom user playlists (`save <name>`, `load <name>`) |
| `/filter <name>` | DSP Filter | Apply FFmpeg audio filters (`bassboost`, `nightcore`, `vaporwave`, `8d`, `pop`) |
| `/stats` | System | View bot performance & RAM usage |
| `/ping` | System | Check bot heartbeat and WebSocket latency |
| `/ytauth` | Admin | YouTube authentication & cookie troubleshooting guide |

---

## Support & EVM Donations

If `aetrna-music` helped you run a fast, ad-free Discord music bot without expensive Lavalink infrastructure, consider supporting the creator!

**EVM Wallet Address** (ETH / BSC / Polygon / Arbitrum / Base / Optimism):
```text
0xA1a4d3F3A49f4514CCEE434Cfc66837A1fFC687d
```

---

## License & Credits

- **License**: Released under the **[Apache License 2.0](LICENSE)**.
- **Creator**: **[zidanaetrna](https://github.com/zidanaetrna)** *(Professional AI Prompter)*
- **Official Bot Artwork**: **[@br_lie](https://github.com/TheBarli)** (`web/artwork.png`)

<div align="center">
  <sub>Built by <b>zidanaetrna</b> using Go & Node.js</sub>
</div>
