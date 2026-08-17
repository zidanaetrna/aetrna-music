# PANDUAN PENGGUNAAN & HASIL AUDIT QA LENGKAP: AETRNA-MUSIC

---

# BAGIAN 1: PANDUAN PENGGUNAAN LENGKAP & DETAIL (USER & OPERATOR GUIDE)

## 1. Arsitektur Operasional
Project **aetrna-music** beroperasi menggunakan arsitektur *dual-runtime microservice*:
1. **Node.js Voice Engine (`voice-server/server.js`)**: Bertindak sebagai satu-satunya *Discord Gateway WebSocket Client*, bertugas mengelola koneksi suara Discord (Voice UDP/WS dengan enkripsi DAVE E2EE), menerima interaksi Slash Commands, menjalankan decoding audio via FFmpeg / `yt-dlp`, dan mengirim stream audio PCM Opus.
2. **Go Core Bot & Web Server (`cmd/bot/main.go`)**: Bertindak sebagai engine antrean lagu (*Queue Store*), database SQLite (*WAL mode*), integrasi Spotify & LRCLIB Lyrics, HTTP REST API, serta server Web Dashboard (React + TypeScript).

---

## 2. Persiapan & Konfigurasi Bot Discord

### A. Registrasi di Discord Developer Portal
1. Buka **[Discord Developer Portal](https://discord.com/developers/applications)**.
2. Klik **New Application**, beri nama bot (misal: `Aetrna Music`).
3. Masuk ke menu **Bot**:
   - Klik **Reset Token** dan salin bot token Anda (`DISCORD_TOKEN`).
   - Aktifkan **Privileged Gateway Intents**:
     - `PRESENCE INTENT` (Optional)
     - `SERVER MEMBERS INTENT` (Disarankan)
     - `MESSAGE CONTENT INTENT` (Wajib aktif jika bot membutuhkan prefix legacy).
4. Masuk ke menu **OAuth2 -> URL Generator**:
   - Pilih Scopes: `bot`, `applications.commands`.
   - Pilih Bot Permissions:
     - `Send Messages`, `Embed Links`, `Attach Files`, `Use External Emojis`, `Add Reactions`
     - `Connect`, `Speak`, `Use Voice Activity`, `Priority Speaker`
     - `Manage Messages` (Untuk pembersihan pesan / interaksi live lyrics).
   - Salin URL hasil generate dan buka di browser untuk mengundang bot ke Discord Server Anda.

---

## 3. Instalasi & Menjalankan Aplikasi

### Metode 1: Menjalankan via Docker Compose (Metode Produksi)
1. **Salin file konfigurasi environment**:
   ```bash
   cp .env.example .env
   ```
2. **Isi konfigurasi `.env`**:
   ```env
   DISCORD_TOKEN=OTk5...xxxxxxxxxxxxxxx
   ADMIN_KEY=PasswordAdminDashboard123
   DASHBOARD_PASSWORD=PasswordAdminDashboard123
   OWNER_ID=123456789012345678
   PORT=8080
   PREFIX=/
   DEFAULT_VOLUME=1.0
   COOKIES_PATH=./cookies.txt
   DB_PATH=./data/aetrna.db
   CACHE_DIR=./data/cache
   MAX_CACHE_SIZE_MB=5120
   YTDLP_CLIENTS=ios,web,android,tv
   ```
3. **Build UI terlebih dahulu (Penting)**:
   ```bash
   cd web
   npm install
   npm run build
   cd ..
   ```
4. **Jalankan Container**:
   ```bash
   docker compose up -d --build
   ```
5. Akses Web Dashboard pada browser di `http://localhost:8080`.

---

### Metode 2: Menjalankan Manual di Lingkungan Lokal (Development)

#### Prasyarat Sistem:
- **Go**: Versi 1.23+
- **Node.js**: Versi 22+
- **FFmpeg**: Terinstal dan terdaftar di `PATH` sistem (`ffmpeg -version`)
- **yt-dlp**: Terinstal dan terdaftar di `PATH` sistem (`yt-dlp --version`)

#### Langkah Eksekusi:
1. **Instalasi Dependensi Node & Build Frontend**:
   ```bash
   # Install dependensi voice-server
   cd voice-server
   npm install
   cd ..

   # Install & Build Web Dashboard
   cd web
   npm install
   npm run build
   cd ..
   ```

2. **Inisialisasi Environment via CLI Wizard (Opsional)**:
   ```bash
   node bin/cli.js
   ```

3. **Jalankan Voice Server (Terminal 1)**:
   ```bash
   node voice-server/server.js
   ```
   *Voice server akan login ke Discord Gateway dan mendengarkan request internal di port `3005`.*

4. **Jalankan Go Engine & Dashboard (Terminal 2)**:
   ```bash
   go run ./cmd/bot
   ```
   *Go Bot akan menghubungkan database SQLite `./data/aetrna.db`, membuka IPC internal di port `47392`, dan menyajikan Web Dashboard di port `8080`.*

---

## 4. Panduan Penggunaan Discord Slash Commands

| Command | Kategori | Format & Opsi | Deskripsi Penggunaan |
|---|---|---|---|
| `/play` | Pemutaran | `/play query:<judul / URL>` | Memutar atau memasukkan lagu ke antrean. Menerima query teks pencarian YouTube, link video/playlist YouTube, atau link Spotify track/playlist. |
| `/search` | Pemutaran | `/search query:<judul lagu>` | Mencari 5 lagu teratas di YouTube dan menampilkan *interactive dropdown select menu* untuk memilih lagu. |
| `/pause` | Kontrol | `/pause` | Menjeda pemutaran lagu yang sedang berjalan. |
| `/resume` | Kontrol | `/resume` | Melanjutkan kembali lagu yang sedang dijeda. |
| `/skip` | Kontrol | `/skip` | Melakukan skip lagu. Jika pengguna adalah requester/admin/pendengar tunggal, lagu langsung dilewati; jika banyak pendengar, mengaktifkan mekanisme *vote-skip*. |
| `/stop` | Kontrol | `/stop` | Menghentikan audio, mengosongkan antrean, dan memutuskan bot dari voice channel. |
| `/queue` | Antrean | `/queue` | Menampilkan daftar antrean lagu saat ini dalam format tabel embed. |
| `/nowplaying` | Status | `/nowplaying` | Menampilkan kartu interaktif lagu yang sedang diputar lengkap dengan progress bar, metadata, dan tombol kontrol interaktif (Pause, Skip, Stop, Favorite). |
| `/lyrics` | Lirik | `/lyrics` | Mengambil lirik lagu yang tersinkronisasi (*synced LRC*) secara real-time baris per baris. |
| `/filter` | DSP Audio | `/filter name:<nama_filter>` | Mengaktifkan filter equalizer FFmpeg on-the-fly (`bassboost`, `nightcore`, `vaporwave`, `8d`, `pop`, `off`). |
| `/favorite` | Database | `/favorite` | Menyimpan lagu yang sedang diputar ke daftar favorit user di SQLite. |
| `/favorites` | Database | `/favorites` | Menampilkan 15 daftar lagu favorit milik user yang memanggil command. |
| `/stats` | Sistem | `/stats` | Menampilkan penggunaan memori RAM, jumlah goroutine, dan uptime bot. |
| `/ping` | Sistem | `/ping` | Mengecek responsivitas interaksi webhook bot. |
| `/language` | Konfigurasi | `/language lang:<en/id/jp>` | Mengubah bahasa respons bot untuk server bersangkutan (*Admin Only*). |
| `/ytauth` | Admin | `/ytauth` | Menampilkan panduan setup `cookies.txt` untuk mengatasi pembatasan video age-restricted (*Owner Only*). |

---

## 5. Panduan Penggunaan Web Dashboard

1. **Autentikasi**:
   - Buka `http://localhost:8080` (atau IP/Domain server Anda).
   - Masukkan password yang dikonfigurasi di file `.env` (variabel `ADMIN_KEY` / `DASHBOARD_PASSWORD`).
   - Token sesi disimpan di `localStorage` (`aetrna_token`) dan HttpOnly Cookie `aetrna_session`.
2. **Navigasi Dashboard**:
   - **Sidebar**: Berisi navigasi tab (*Overview*, *Queue Player*, *DSP Filters*, *Logs*, *Favorites*, *Settings*) dan tombol toggle collapse (shortcut `Ctrl + K` untuk pencarian).
   - **TopBar**: Menampilkan breadcrumb status dan *Target Guild Selector* untuk beralih kontrol antar server Discord.
   - **Tab Overview**:
     - *Telemetry Metrics*: Menampilkan status sinkronisasi WebSocket aktif, total active guilds, penggunaan RAM, dan bot uptime.
     - *Now Playing Card*: Menampilkan status stream, thumbnail artwork, metadata lagu, visualisasi equalizer bars, tombol kontrol pemutaran (*Pause*, *Skip*, *Stop*, *Kick/Disconnect*), dan slider volume.
     - *Queue List*: Menampilkan urutan antrean lagu yang sedang aktif.
3. **Multi-Language Switcher**:
   - Dapat diakses melalui selector bahasa pada menu pengaturan untuk beralih antara Bahasa Inggris, Bahasa Indonesia, dan Bahasa Jepang.

---

## 6. Konfigurasi YouTube Cookies (`cookies.txt`)
Untuk mencegah blokir YouTube 403 Forbidden atau video *Age-Restricted*:
1. Install ekstensi browser (misal: *Get cookies.txt LOCALLY* di Chrome/Firefox).
2. Login ke akun YouTube standar.
3. Export cookies dalam format Netscape dan simpan sebagai file `cookies.txt` di root direktori project.
4. Restart bot agar audio engine membaca header cookie secara otomatis.

---
---

# BAGIAN 2: HASIL AUDIT TESTING QA PROFESIONAL (SECURITY, UI/UX, BACKEND, INFRASTRUCTURE)

```
========================================================================================
                        AETRNA-MUSIC QA AUDIT & BENCHMARK REPORT
========================================================================================
Auditor Role     : Lead Full-Stack QA & Systems Security Engineer
Target Repository: https://github.com/Akrmfdhl/aetrna-music (Forked from zidanaetrna/aetrna-music)
Audit Scope      : Security, UI/UX, Backend Audio & Queue Engine, Infrastructure & DevOps
Audit Verdict    : PARTIALLY PRODUCTION READY (Memerlukan Patching Keamanan & Integrasi Frontend)
========================================================================================
```

---

## PILLAR 1: SECURITY AUDIT & VULNERABILITY ASSESSMENT

### 1.1 Critical Vulnerability: Mismatch Variabel Konfigurasi Password Dashboard
- **Severity**: `CRITICAL` (CVSS 8.6)
- **Lokasi Kode**:
  - `config/config.go` (Baris 34)
  - `internal/bot/auth.go` (Baris 24–30)
  - `.env.example` (Baris 17) & `bin/cli.js` (Baris 69)
- **Temuan Root Cause**:
  Pada `.env.example` dan `bin/cli.js`, variabel didefinisikan sebagai `DASHBOARD_PASSWORD`. Namun pada `config/config.go`:
  ```go
  AdminKey: getEnv("ADMIN_KEY", ""),
  ```
  `config.go` hanya membaca `ADMIN_KEY`, tanpa fallback ke `DASHBOARD_PASSWORD`.
  Akibatnya, jika operator menyetel `DASHBOARD_PASSWORD=Rahasia123` di `.env`, nilai `cfg.AdminKey` tetap kosong (`""`). Di `internal/bot/auth.go`:
  ```go
  if pwd == "" || pwd == "your-super-secret-admin-key-12345" {
      b := make([]byte, 8)
      _, _ = rand.Read(b)
      pwd = hex.EncodeToString(b)
      log.Printf("[INFO] [Auth] No DASHBOARD_PASSWORD specified. Generated random Dashboard Password: %s", pwd)
  }
  ```
  Sistem akan mengabaikan password operator dan men-generate random token yang berbeda setiap kali bot di-restart.
- **Rekomendasi**: Ubah `config/config.go` agar membaca `DASHBOARD_PASSWORD` jika `ADMIN_KEY` kosong:
  `AdminKey: getEnv("ADMIN_KEY", getEnv("DASHBOARD_PASSWORD", ""))`.

---

### 1.2 High Vulnerability: Fallback Autentikasi Frontend (Client-Side Auth Bypass Mock)
- **Severity**: `HIGH` (CVSS 7.5)
- **Lokasi Kode**: `web/src/components/LoginModal.tsx` (Baris 36–40)
- **Temuan Root Cause**:
  Pada komponen login React:
  ```typescript
  } catch (err) {
      // Local dev mode fallback
      onLoginSuccess('local_dev_token');
      showToast(t('toast_auth_success'), 'success');
  }
  ```
  Jika backend API down, terjadi network timeout, atau server mengembalikan network error, blok `catch` secara otomatis memberikan token `'local_dev_token'` dan meloloskan pengguna ke dashboard. Walaupun request API `/api/status` akan gagal (401), antarmuka dashboard tetap terbuka dan membingungkan operator.
- **Rekomendasi**: Hapus bypass `catch` pada mode produksi dan tampilkan pesan error koneksi yang jelas.

---

### 1.3 Medium Vulnerability: SSE & WebSocket Telemetry Endpoint Tanpa Autentikasi
- **Severity**: `MEDIUM` (CVSS 5.3)
- **Lokasi Kode**: `internal/bot/server.go` (Baris 141 & 218)
- **Temuan Root Cause**:
  Endpoint `/api/status` dan `/api/control` dilindungi oleh `auth.RequireAuth`. Namun:
  - `/api/events` (Server-Sent Events)
  - `/api/ws` (WebSocket Telemetry)
  tidak dibungkus middleware `RequireAuth`, dan `upgrader.CheckOrigin` mengembalikan `true` secara permanen. Siapa pun dapat mengakses metrik sistem internal (RAM, active guilds, uptime) dari domain eksternal tanpa token.
- **Rekomendasi**: Pasang validasi token sesi pada handshake WebSocket (`/api/ws`) dan SSE stream.

---

### 1.4 Medium Vulnerability: Unauthenticated Internal IPC & SSRF Vector
- **Severity**: `MEDIUM` (CVSS 6.1)
- **Lokasi Kode**:
  - `voice-server/server.js` (Port 3005: `/join-and-play`, `/stop`, `/test-voice`)
  - `internal/bot/bot.go` (Port 47392: `/internal/interaction`, `/internal/track-end`)
- **Temuan Root Cause**:
  Port internal `3005` dan `47392` bind ke `127.0.0.1`. Jika terdapat service lain dalam host/container atau celah SSRF, request dapat langsung mengontrol voice connection bot tanpa autentikasi / secret bearer token bersama.
- **Rekomendasi**: Implementasikan shared internal secret header (misal: `X-Internal-Secret`) antara Go dan Node.js.

---

### 1.5 Low Vulnerability: Memory Leak pada Session Map
- **Severity**: `LOW` (CVSS 3.7)
- **Lokasi Kode**: `internal/bot/auth.go` (Baris 56–84)
- **Temuan Root Cause**:
  Token login disimpan pada `a.sessions[token]`. Pembersihan token kedaluwarsa (`delete(a.sessions, token)`) hanya terjadi saat token spesifik tersebut divalidasi kembali setelah masa berlaku habis. Jika ada banyak percobaan login yang menghasilkan token yang tidak pernah digunakan lagi, map `sessions` akan terus bertambah di memori (*unbounded growth*).
- **Rekomendasi**: Tambahkan periodic cleanup worker dengan `time.Ticker` untuk menghapus sesi yang expired.

---

## PILLAR 2: UI/UX & FRONTEND AUDIT

### 2.1 Critical UX Issue: Komponen Dashboard Menggunakan Data Statis (Mock Data)
- **Severity**: `HIGH`
- **Lokasi Kode**:
  - `web/src/components/TopBar.tsx` (Baris 46–49)
  - `web/src/components/OverviewTab.tsx` (Baris 48, 70, 91–93, 166–205)
  - `web/src/App.tsx` (Baris 78–82, 122–126, 138–140)
- **Temuan Detail**:
  1. **Guild Selector**: List guild di `TopBar.tsx` di-hardcode (`Aetrna Lounge`, `Chill Vibes & Gaming`, `Code & Music Squad`, `Anime Music Hub`). Dropdown tidak mengambil data guild riil dari API backend (`/api/status`).
  2. **Queue Table & Now Playing**: Tabel antrean dan judul lagu di `OverviewTab.tsx` menampilkan teks statis dummy (*"Continuous — Celestial Resonance"*, *"Synthwave Horizon"*), bukan data dinamis dari SQLite / `q.Songs`.
  3. **Tab DSP & Logs**: Tab logs menampilkan baris teks statis yang tidak mencerminkan log engine riil; tombol di tab DSP tidak memiliki event listener untuk memanggil `/api/control`.
- **Dampak UX**: Pengguna yang membuka web dashboard melihat antarmuka yang sangat elegan dan modern, namun data pemutaran tidak sinkron dengan aktivitas bot Discord sesungguhnya.

---

### 2.2 Functional Bug: Format Output Uptime pada WebSocket Telemetry
- **Severity**: `MEDIUM`
- **Lokasi Kode**: `internal/bot/server.go` (Baris 237)
- **Temuan Root Cause**:
  Pada handler `/api/ws`, field `uptime` diisi dengan data memori sistem:
  ```go
  "uptime": fmt.Sprintf("%d MB", m.Sys/1024/1024),
  ```
  Ini menyebabkan nilai uptime pada payload JSON bertuliskan format megabyte memori (misal: `"35 MB"`), bukan format durasi waktu.

---

### 2.3 UX Excellence & Positives
- Desain antarmuka *Cloudflare Dark Charcoal* dengan palet Deep Emerald Green (`#10B981`) sangat rapi, konsisten, dan berkelas tinggi.
- Responsive layout mendukung collapsible sidebar (64px vs 250px) dan interaksi keyboard shortcut (`Ctrl + K`).
- Animasi transisi CSS smooth dan tidak merusak layout flow.
- Sistem i18n (Inggris, Indonesia, Jepang) diimplementasikan dengan Context API yang solid.

---

## PILLAR 3: BACKEND & AUDIO ENGINE AUDIT

### 3.1 Architecture Review: Single Gateway + Microservice IPC Model
- **Penilaian**: `EXCELLENT`
- **Analisis**:
  Pemisahan Discord Gateway ke Node.js `@discordjs/voice` dan logika bisnis ke Go mengeliminasi kebutuhan Java Lavalink server terpisah.
  - Node.js langsung mengeksekusi `deferReply()` / `deferUpdate()` dalam 3 detik pertama Discord Gateway SLA, lalu meneruskan interaksi ke Go secara asinkron (Go memiliki waktu 15 menit melalui webhook followup).
  - Voice handshake di Node.js mengintegrasikan protokol DAVE E2EE (`@snazzah/davey`) dan libsodium secara tepat.

---

### 3.2 Performance Bottleneck: Spotify Playlist Sequential yt-dlp Queueing
- **Severity**: `MEDIUM`
- **Lokasi Kode**: `internal/bot/bot.go` (Baris 321–333)
- **Temuan Root Cause**:
  Saat user memutar playlist Spotify berisi 50 lagu:
  Lagu pertama diselesaikan secara sinkron, sedangkan 49 lagu sisanya diproses dalam goroutine menggunakan loop sekuensial `SearchYouTube(tQuery, 1, ...)` yang memanggil binary `yt-dlp` satu per satu secara berurutan.
  Meskipun `ytdlpSemaphore` membatasi konkurensi maksimum 12 proses, 49 pemanggilan `yt-dlp` sekuensial memerlukan waktu 60–120 detik untuk menyelesaikan seluruh antrean playlist dan berisiko memicu rate limit IP dari YouTube.
- **Rekomendasi**: Gunakan batching internal atau query YouTube Music API scraper lightweight untuk metadata playlist sebelum stream URL di-resolve on-demand.

---

### 3.3 Rate Limit Risk: Live Lyrics Background Updater
- **Severity**: `MEDIUM`
- **Lokasi Kode**: `internal/bot/bot.go` (Baris 740–780)
- **Temuan Root Cause**:
  Untuk fitur lirik sinkron (*synced LRC*), bot menjalankan goroutine dengan interval update antara 1000ms hingga 2500ms via `b.session.FollowupMessageEdit()`.
  Jika ada 3–5 guild aktif yang bersamaan memutar lagu dengan live lyrics aktif, webhook endpoint Discord dapat terkena HTTP 429 (*Too Many Requests*).
- **Rekomendasi**: Naikkan minimum sleep interval menjadi 2500ms–3000ms dan batasi edit hanya saat baris lirik benar-benar berganti (*dirty check*).

---

### 3.4 Audio Fallback Pipeline: Direct FFmpeg to yt-dlp Pipe
- **Penilaian**: `VERY ROBUST`
- **Lokasi Kode**: `voice-server/server.js` (Baris 576–642)
- **Analisis**:
  Audio engine mengutamakan direct stream FFmpeg ke URL audio YouTube dengan buffer reconnection. Jika stream mengalami `403 Forbidden` (misal token kedaluwarsa atau IP restriction), sistem secara otomatis beralih (*fallback*) ke proses `yt-dlp` piping langsung ke `ffmpeg.stdin` tanpa memutus voice session Discord.

---

## PILLAR 4: INFRASTRUCTURE & DEVOPS AUDIT

### 4.1 Build Failure Vector: Dockerfile Multi-Stage Tidak Mengompilasi Web UI
- **Severity**: `HIGH`
- **Lokasi Kode**: `Dockerfile` (Baris 3–10)
- **Temuan Root Cause**:
  Pada Stage 1 `Dockerfile`:
  ```dockerfile
  FROM golang:1.23-alpine AS go-builder
  WORKDIR /build
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  RUN CGO_ENABLED=0 GOOS=linux go build -o aetrna-bot ./cmd/bot
  ```
  Go mengompilasi `web/embed.go` yang menggunakan `//go:embed all:dist`. Di repositori git, folder `web/dist` hanya berisi `.gitkeep`.
  `Dockerfile` tidak memiliki stage Node.js builder untuk menjalankan `npm run build` di folder `web` sebelum Go mengompilasi binary.
  Akibatnya, jika operator menjalankan `docker compose up --build` dari clone baru tanpa build lokal terlebih dahulu, file embedded `dist` akan kosong dan Web Dashboard akan mengembalikan error 404 saat dibuka.
- **Rekomendasi**: Tambahkan Stage Frontend Builder di `Dockerfile`:
  ```dockerfile
  # Stage 0: Build React Frontend
  FROM node:22-alpine AS web-builder
  WORKDIR /web
  COPY web/package*.json ./
  RUN npm install
  COPY web/ ./
  RUN npm run build

  # Stage 1: Build Go Binary (Copy dist dari Stage 0)
  FROM golang:1.23-alpine AS go-builder
  WORKDIR /build
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  COPY --from=web-builder /web/dist ./web/dist
  RUN CGO_ENABLED=0 GOOS=linux go build -o aetrna-bot ./cmd/bot
  ```

---

### 4.2 Security Hardening: Container Berjalan Sebagai User `root`
- **Severity**: `LOW`
- **Lokasi Kode**: `Dockerfile` & `entrypoint.sh`
- **Temuan Root Cause**:
  Container berjalan menggunakan privileges `root`. Jika terjadi eksploitasi pada binary native `yt-dlp` atau `ffmpeg`, penyerang memiliki akses root di dalam container.
- **Rekomendasi**: Tambahkan non-root system user (`adduser -S aetrna`) pada Stage 2 dan ubah kepemilikan direktori `/app/data`.

---

### 4.3 Process Supervisor Stability: `entrypoint.sh`
- **Penilaian**: `GOOD`
- **Lokasi Kode**: `entrypoint.sh`
- **Analisis**:
  Penggunaan signal trap (`trap cleanup SIGTERM SIGINT EXIT`) dan `wait -n "$NODE_PID" "$GO_PID"` menangani graceful shutdown secara tepat jika salah satu proses crash atau menerima signal kill dari Docker engine.

---

# MATRIKS TEMUAN & ACTION PLAN QA

| ID | Kategori | Severity | Komponen | Deskripsi Masalah | Solusi Tindakan |
|---|---|---|---|---|---|
| **SEC-01** | Security | `CRITICAL` | `config.go` / `auth.go` | Variabel `DASHBOARD_PASSWORD` tidak dibaca oleh Go backend (`ADMIN_KEY` mismatch). | Tambahkan fallback pembacaan `DASHBOARD_PASSWORD` di `config.go`. |
| **SEC-02** | Security | `HIGH` | `LoginModal.tsx` | Fallback catch login mengizinkan login bypass palsu jika network offline. | Hapus mock token fallback di production code. |
| **SEC-03** | Security | `MEDIUM` | `server.go` | Endpoint `/api/events` & `/api/ws` terbuka tanpa auth & CORS wildcard. | Terapkan token session check pada handshake WS/SSE. |
| **SEC-04** | Security | `MEDIUM` | `server.js` & `bot.go` | IPC Port 3005 & 47392 unauthenticated. | Pasang shared header secret antar microservice. |
| **UI-01** | UI/UX | `HIGH` | `TopBar.tsx` & `OverviewTab.tsx` | Dropdown Guild, Queue list, & Tab Logs menggunakan data statis/dummy. | Sambungkan state komponen React ke endpoint API `/api/status` & `/api/queue`. |
| **UI-02** | UI/UX | `MEDIUM` | `server.go` (WS) | Payload `uptime` mengirim data MB memori (`m.Sys`). | Ganti formatting uptime dengan durasi waktu riil (`time.Since`). |
| **BE-01** | Backend | `MEDIUM` | `bot.go` (Spotify) | Loop Spotify playlist menembak `yt-dlp` sekuensial tanpa batching. | Batasi limit batching / caching metadata sebelum streaming. |
| **BE-02** | Backend | `MEDIUM` | `bot.go` (Lyrics) | Live lyrics updater berpotensi memicu Discord HTTP 429 rate limit. | Terapkan dirty check (hanya edit jika baris lirik berubah) & naikkan interval. |
| **INF-01**| Infra | `HIGH` | `Dockerfile` | Multi-stage build tidak menyertakan build Vite frontend sebelum Go compile. | Tambahkan Stage Node.js Frontend Builder di `Dockerfile`. |
| **INF-02**| Infra | `LOW` | `Dockerfile` | Eksekusi runtime container menggunakan user `root`. | Buat dedicated non-root user `aetrna` di Alpine image. |

---

# KESIMPULAN AKHIR TESTER QA

Arsitektur inti audio engine (Go microservice + Node.js Gateway + FFmpeg DSP + DAVE E2EE) dirancang dengan performa sangat tinggi, tangguh dalam menangani stream YouTube/Spotify, dan bebas dari ketergantungan Lavalink. 

Untuk mencapai standar **Production Grade 100%**, langkah prioritas yang harus diperbaiki adalah:
1. Menyelaraskan parsing env `DASHBOARD_PASSWORD` di `config.go`.
2. Menghubungkan state React UI ke endpoint backend riil (menggantikan data mock).
3. Menambahkan stage build frontend Vite di dalam `Dockerfile`.

---

# BAGIAN 3: TEMUAN MASALAH BAWAAN REPO & PERBAIKAN DOCKER (DOCKER BUGFIXES SUMMARY)

1. **Fix `entrypoint.sh` Line Ending (CRLF -> LF)**: Menghilangkan error `exec /app/entrypoint.sh: no such file or directory` pada Alpine Linux.
2. **Fix `voice-server/server.js` Port Collision**: Mengalihkan port Node.js ke `VOICE_PORT=3005` agar tidak menabrak port dashboard `PORT=8080`.
3. **Fix `docker-compose.yml` Read-Only Cookies Volume Mount**: Menghapus flag `:ro` pada mount `cookies.txt` dan menambahkan validasi ukuran file sebelum memanggil flag `--cookies` di `yt-dlp` untuk mencegah crash `OSError: Read-only file system`.
4. **Fix Linter Warning `music.go`**: Menyematkan parameter `ytdlpClients` pada `searchYouTubeFallback()` di `internal/commands/music.go`.
