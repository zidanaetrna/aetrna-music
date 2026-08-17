# YouTube Cookies Setup Guide

YouTube frequently blocks data center IP ranges, imposes rate limits (HTTP 429), or restricts age-gated audio streams. Supplying a valid `cookies.txt` file ensures uninterrupted audio playback.

> **Critical Filename Note**: The file **MUST** be named exactly `cookies.txt` at the project root (same folder as `docker-compose.yml` and `.env.example`). The default env var `COOKIES_PATH=./cookies.txt` and `docker-compose.yml` volume mount both expect this exact name, not `youtube_cookies.txt`.

---

## Step-by-Step Cookie Export Guide

### 1. Install Browser Extension
Install a trusted netscape cookie exporter extension for your browser:
- **Chrome / Brave / Edge**: Install **[Get cookies.txt LOCALLY](https://chromewebstore.google.com/detail/get-cookiestxt-locally/cclelndahbckbenkjhflpdbgdldlbecc)** from the Chrome Web Store.
- **Firefox**: Install **[Get cookies.txt LOCALLY](https://addons.mozilla.org/en-US/firefox/addon/get-cookies-txt-locally/)** from Mozilla Add-ons.

### 2. Log in to YouTube
Open `https://youtube.com` in your browser and ensure you are logged into an active account. Browsing a couple of videos and leaving the tab open for a minute helps populate all cookie domains (`.youtube.com`, `.google.com`, SID, HSID, SSID, APISID, SAPISID, etc.).

### 3. Export Cookies File
1. Click the extension icon in your browser toolbar while on `youtube.com`.
2. Click **Export** (or **Download**) to save the cookie file — make sure it is in **Netscape format**.
3. **Rename the downloaded file** to `cookies.txt` exactly.

### 4. Verify Minimum File Size
The exported file must be **larger than ~100 bytes** (typically around 3–8 KB for a logged-in YouTube account). The Go and Node.js IPC layers both enforce a `size > 100 bytes` check before passing `--cookies` to yt-dlp. Files smaller than this threshold are silently skipped to avoid `yt-dlp -1 exit code` errors.

### 5. Place in Project Directory
Place `cookies.txt` in your project root directory (the same folder containing `docker-compose.yml`):

```
your-project/
├── docker-compose.yml
├── .env.example
└── cookies.txt        ← here
```

### 6. Configure `.env` (optional override)
If you want to place the file elsewhere, override the path in your `.env` file:
```env
COOKIES_PATH=./somewhere/else/my-youtube-cookies.txt
```
If you leave this unset, the default `./cookies.txt` is used automatically.

### 7. Docker Compose Mount Behavior
`docker-compose.yml` automatically mounts your local `./cookies.txt` into `/app/cookies.txt` inside the container as **read+write** (not `:ro`). This is intentional: yt-dlp updates cookie expiry timestamps and refreshes session tokens on the fly during playback. A read-only mount causes `yt-dlp [Errno 30] Read-only file system` errors.
