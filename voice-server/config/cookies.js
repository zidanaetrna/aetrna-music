const fs = require('fs');
const path = require('path');

function getAbsoluteCookiesPath() {
    try {
        const possiblePaths = [
            process.env.COOKIES_PATH,
            process.env.COOKIES_PATH ? path.resolve(process.cwd(), process.env.COOKIES_PATH) : null,
            path.join(__dirname, '../../cookies.txt'),
            path.join(__dirname, '../cookies.txt'),
            '/opt/aetrna-music/prod/cookies.txt',
            '/opt/aetrna-music/cookies.txt',
            './cookies.txt',
            '/app/cookies.txt'
        ].filter(Boolean);

        for (const p of possiblePaths) {
            if (fs.existsSync(p)) {
                return p;
            }
        }
    } catch (_) {}
    return null;
}

function getCacheDir() {
    try {
        const dir = path.join(__dirname, '../../.cache/yt-dlp');
        if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
        return dir;
    } catch (_) {}
    return null;
}

function getCookieHeaderString() {
    try {
        const cookiesFile = getAbsoluteCookiesPath();
        if (!cookiesFile) return '';
        const content = fs.readFileSync(cookiesFile, 'utf8');
        if (!content) return '';

        const lines = content.split('\n');
        const cookiePairs = [];

        for (const line of lines) {
            const trimmed = line.trim();
            if (!trimmed) continue;
            if (trimmed.startsWith('#') && !trimmed.startsWith('#HttpOnly_')) continue;
            const cleanLine = trimmed.startsWith('#HttpOnly_') ? trimmed.substring(10) : trimmed;
            const parts = cleanLine.split(/\s+/);
            if (parts.length >= 7) {
                const domain = parts[0].trim();
                const name = parts[5].trim();
                const value = parts[6].trim();
                if ((domain.includes('youtube') || domain.includes('googlevideo') || domain.includes('.com')) && name && value) {
                    cookiePairs.push(`${name}=${value}`);
                }
            }
        }
        return cookiePairs.join('; ');
    } catch (e) {
        return '';
    }
}

module.exports = {
    getAbsoluteCookiesPath,
    getCacheDir,
    getCookieHeaderString
};
