# # aetrna-music 🎵

A high-performance, modular Discord music bot written in **Golang**. Featuring interactive UI embeds, dual Slash (`/`) and Legacy (`!`) command support, SQLite persistence for user favorites & custom collections, dynamic FFmpeg audio DSP filters, and a built-in LRU disk audio caching engine.

---

## 🌟 Key Features

- **⚡ Ultra-Fast & Lightweight Engine**: Consumes only **15–30 MB RAM** per instance (compared to 200MB+ in Node.js).
- **🎨 Interactive UI Components**:
  - Full-width album cover banner cards.
  - Interactive Now Playing Control Bar (`Pause`, `Skip`, `Prev`, `Loop`, `Shuffle`, `Vol+`, `Vol-`, `Filter`, `Favorite`, `Stop`).
  - Search dropdown Select Menu.
  - Paginated Queue table with visual progress bar.
- **📁 SQLite Persistence**:
  - Saved favorites (`/favorite`, `/favorites`).
  - User custom collections (`/collection save <name>`, `/collection load <name>`).
- **🎛️ Dynamic Audio DSP Filters**:
  - Bassboost, Nightcore, Vaporwave, 8D Audio, Pop.
- **🚀 CI/CD & VPS Staging Pipeline**:
  - Automated build checks and SSH deployment to VPS (`staging` and `main` branches).

---

## 🛠️ System Requirements

- **Go**: 1.22+
- **ffmpeg**: Installed and added to system `PATH`.
- **yt-dlp**: Installed and added to system `PATH`.

---

## 🚀 Quick Start (Local Run)

1. **Clone the repository**:
   ```bash
   git clone https://github.com/zidanaetrna/aetrna-music.git
   cd aetrna-music
   ```

2. **Configure environment variables**:
   ```bash
   cp .env.example .env
   ```
   Fill in your `DISCORD_TOKEN` in `.env`.

3. **Run the bot**:
   ```bash
   go run ./cmd/bot
   ```

---

## 📜 Command List

| Command | Category | Description |
|---|---|---|
| `/play <query>` | Playback | Search or queue track/playlist |
| `/search <query>` | Playback | Search tracks with interactive dropdown menu |
| `/pause` / `/resume` | Playback | Pause or resume audio playback |
| `/skip` | Playback | Skip to the next track |
| `/stop` | Playback | Stop playback and clear queue |
| `/queue` | Queue | View paginated queue table |
| `/nowplaying` | Queue | View Now Playing card with controls |
| `/favorite` | Favorites | Save current song to your SQLite favorites |
| `/favorites` | Favorites | View your favorite songs |
| `/filter <name>` | DSP Filter | Apply FFmpeg audio equalizer filter |
| `/stats` | System | View bot performance & RAM usage |
| `/ping` | System | Check bot heartbeat latency |
| `/ytauth` | Admin | YouTube authentication & cookies guide |

---

## 📄 License

MIT License. Created by [zidanaetrna](https://github.com/zidanaetrna).
