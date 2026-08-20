const http = require('http');

const GITHUB_REPO = 'zidanaetrna/aetrna-music';
const GITHUB_RELEASES_URL = `https://api.github.com/repos/${GITHUB_REPO}/releases/latest`;
const RELEASES_PAGE_URL = `https://github.com/${GITHUB_REPO}/releases`;
const DEFAULT_CHECK_INTERVAL_MS = 20 * 60 * 1000;
const REQUEST_TIMEOUT_MS = 8 * 1000;

function parseSemver(raw) {
    let s = String(raw || '').trim();
    s = s.replace(/^[vV]/, '');
    let pre = '';
    const idx = s.search(/[-+]/);
    if (idx !== -1) {
        if (s[idx] === '-') pre = s.substring(idx + 1);
        s = s.substring(0, idx);
    }
    const parts = s.split('.');
    if (parts.length !== 3) return null;
    const maj = parseInt(parts[0], 10);
    const min = parseInt(parts[1], 10);
    const pat = parseInt(parts[2], 10);
    if (isNaN(maj) || isNaN(min) || isNaN(pat)) return null;
    return { major: maj, minor: min, patch: pat, pre };
}

function compareSemver(a, b) {
    const sa = parseSemver(a);
    const sb = parseSemver(b);
    if (!sa || !sb) return 0;
    if (sa.major < sb.major) return -1;
    if (sa.major > sb.major) return 1;
    if (sa.minor < sb.minor) return -1;
    if (sa.minor > sb.minor) return 1;
    if (sa.patch < sb.patch) return -1;
    if (sa.patch > sb.patch) return 1;
    if (sa.pre === '' && sb.pre !== '') return 1;
    if (sa.pre !== '' && sb.pre === '') return -1;
    return 0;
}

function getenvBool(key, def) {
    const v = String(process.env[key] || '').trim();
    if (v === '') return def;
    try { return JSON.parse(v.toLowerCase()); } catch (_) { return def; }
}

async function getRemoteLatest(version) {
    if (getenvBool('DISABLE_VERSION_CHECK', false)) {
        return { tag: version, changelogURL: RELEASES_PAGE_URL, err: null };
    }
    return new Promise((resolve) => {
        const timeout = setTimeout(() => {
            resolve({ tag: version, changelogURL: RELEASES_PAGE_URL, err: new Error('request timeout') });
        }, REQUEST_TIMEOUT_MS);
        const req = http.request(GITHUB_RELEASES_URL, {
            method: 'GET',
            headers: { 'User-Agent': 'aetrna-music-version-checker' }
        }, (res) => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', () => {
                clearTimeout(timeout);
                try {
                    if (res.statusCode !== 200) {
                        return resolve({ tag: version, changelogURL: RELEASES_PAGE_URL, err: new Error(`HTTP ${res.statusCode}`) });
                    }
                    const json = JSON.parse(data);
                    const tag = json.tag_name || version;
                    const url = json.html_url || RELEASES_PAGE_URL;
                    resolve({ tag, changelogURL: url, err: null });
                } catch (e) {
                    resolve({ tag: version, changelogURL: RELEASES_PAGE_URL, err: e });
                }
            });
        });
        req.on('error', (e) => {
            clearTimeout(timeout);
            resolve({ tag: version, changelogURL: RELEASES_PAGE_URL, err: e });
        });
        req.end();
    });
}

let voiceVersionCache = { tag: null, changelogURL: RELEASES_PAGE_URL, checkedAt: 0 };

async function runVoiceVersionCheck(version) {
    if (getenvBool('DISABLE_VERSION_CHECK', false)) return;
    const now = Date.now();
    let latest, changelogURL, err;
    if (voiceVersionCache.tag && (now - voiceVersionCache.checkedAt) < DEFAULT_CHECK_INTERVAL_MS) {
        latest = voiceVersionCache.tag;
        changelogURL = voiceVersionCache.changelogURL;
        err = null;
    } else {
        const res = await getRemoteLatest(version);
        latest = res.tag;
        changelogURL = res.changelogURL;
        err = res.err;
        voiceVersionCache = { tag: latest, changelogURL, checkedAt: now };
    }
    if (err) {
        console.warn(`[WARN] [VoiceServer] [VersionCheck] Remote latest lookup failed: ${err.message}`);
        return;
    }
    const cmp = compareSemver(version, latest);
    if (cmp < 0) {
        console.warn(`[WARN] [VoiceServer] [VersionCheck] This instance: ${version} — latest published release: ${latest}. Upgrade recommended: ${changelogURL}`);
    } else {
        console.log(`[INFO] [VoiceServer] [VersionCheck] Running ${version}. Latest published release: ${latest}.`);
    }
}

function initVersionCheck(version) {
    if (!getenvBool('DISABLE_VERSION_CHECK', false)) {
        setTimeout(() => { runVoiceVersionCheck(version).catch(() => {}); }, 2000);
        setInterval(() => { runVoiceVersionCheck(version).catch(() => {}); }, DEFAULT_CHECK_INTERVAL_MS);
    } else {
        console.log('[INFO] [VoiceServer] [VersionCheck] Version checking is disabled via DISABLE_VERSION_CHECK.');
    }
}

module.exports = {
    parseSemver,
    compareSemver,
    initVersionCheck
};
