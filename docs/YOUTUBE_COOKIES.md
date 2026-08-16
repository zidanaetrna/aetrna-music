# YouTube Cookies Setup Guide

YouTube frequently blocks data center IP ranges, imposes rate limits (HTTP 429), or restricts age-gated audio streams. Supplying a valid `youtube_cookies.txt` file ensures uninterrupted audio playback.

---

## Step-by-Step Cookie Export Guide

### 1. Install Browser Extension
Install a trusted netscape cookie exporter extension for your browser:
- **Chrome / Brave / Edge**: Install **[Get cookies.txt LOCALLY](https://chromewebstore.google.com/detail/get-cookiestxt-locally/cclelndahbckbenkjhflpdbgdldlbecc)** from the Chrome Web Store.
- **Firefox**: Install **[Get cookies.txt LOCALLY](https://addons.mozilla.org/en-US/firefox/addon/get-cookies-txt-locally/)** from Mozilla Add-ons.

### 2. Log in to YouTube
Open `https://youtube.com` in your browser and ensure you are logged into an active account.

### 3. Export Cookies File
1. Click the extension icon in your browser toolbar while on `youtube.com`.
2. Click **Export** (or **Download**) to save the cookie file.
3. Rename the downloaded file to `youtube_cookies.txt`.

### 4. Place in Project Directory
Place `youtube_cookies.txt` in your project root directory (the same folder containing `docker-compose.yml`).

### 5. Configure `.env`
Set the path in your `.env` file:
```env
YOUTUBE_COOKIES_PATH=./youtube_cookies.txt
```

*Note: If deploying via Docker, `docker-compose.yml` automatically mounts `./youtube_cookies.txt` into `/app/youtube_cookies.txt` inside the container.*
