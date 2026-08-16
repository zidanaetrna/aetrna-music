/* Aetrna's Music Dashboard — Single Page Application Logic */

document.addEventListener('DOMContentLoaded', () => {
    const loginModal = document.getElementById('loginModal');
    const loginForm = document.getElementById('loginForm');
    const passwordInput = document.getElementById('passwordInput');
    const togglePasswordBtn = document.getElementById('togglePasswordBtn');
    const loginErrorText = document.getElementById('loginErrorText');
    const loginError = document.getElementById('loginError');
    const dashboardApp = document.getElementById('dashboardApp');
    const logoutBtn = document.getElementById('logoutBtn');
    const navItems = document.querySelectorAll('.nav-item');
    const tabPages = document.querySelectorAll('.tab-page');

    let sessionToken = localStorage.getItem('aetrna_token') || '';

    // Password Visibility Toggle Handler
    if (togglePasswordBtn) {
        togglePasswordBtn.addEventListener('click', () => {
            const isPassword = passwordInput.type === 'password';
            passwordInput.type = isPassword ? 'text' : 'password';
        });
    }

    // Copy EVM Donation Wallet Handler
    const copyWalletBtn = document.getElementById('copyWalletBtn');
    const donationWalletInput = document.getElementById('donationWalletInput');
    const copyWalletBtnText = document.getElementById('copyWalletBtnText');

    if (copyWalletBtn && donationWalletInput) {
        copyWalletBtn.addEventListener('click', () => {
            navigator.clipboard.writeText(donationWalletInput.value).then(() => {
                if (copyWalletBtnText) copyWalletBtnText.textContent = 'Copied!';
                setTimeout(() => {
                    if (copyWalletBtnText) copyWalletBtnText.textContent = 'Copy';
                }, 2000);
            }).catch(() => {
                donationWalletInput.select();
                document.execCommand('copy');
                if (copyWalletBtnText) copyWalletBtnText.textContent = 'Copied!';
                setTimeout(() => {
                    if (copyWalletBtnText) copyWalletBtnText.textContent = 'Copy';
                }, 2000);
            });
        });
    }

    // Tab Navigation Switcher
    navItems.forEach(item => {
        item.addEventListener('click', () => {
            const targetTab = item.getAttribute('data-tab');
            navItems.forEach(n => n.classList.remove('active'));
            tabPages.forEach(p => p.classList.remove('active'));
            item.classList.add('active');
            document.getElementById(`tab-${targetTab}`).classList.add('active');
        });
    });

    // Login Form Handler
    if (loginForm) {
        loginForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            if (loginError) loginError.classList.add('hidden');
            const password = passwordInput.value;

            try {
                const res = await fetch('/api/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ password })
                });

                if (!res.ok) {
                    if (loginErrorText) loginErrorText.textContent = 'Invalid password. Please check your .env configuration.';
                    if (loginError) loginError.classList.remove('hidden');
                    return;
                }

                const data = await res.json();
                sessionToken = data.token;
                localStorage.setItem('aetrna_token', sessionToken);
                showDashboard();
            } catch (err) {
                if (loginErrorText) loginErrorText.textContent = 'Connection error to Aetrna Bot API server.';
                if (loginError) loginError.classList.remove('hidden');
            }
        });
    }

    // Logout Handler
    if (logoutBtn) {
        logoutBtn.addEventListener('click', () => {
            sessionToken = '';
            localStorage.removeItem('aetrna_token');
            showLogin();
        });
    }

    function showLogin() {
        if (loginModal) loginModal.classList.remove('hidden');
        if (dashboardApp) dashboardApp.classList.add('hidden');
    }

    function showDashboard() {
        if (loginModal) loginModal.classList.add('hidden');
        if (dashboardApp) dashboardApp.classList.remove('hidden');
        fetchDashboardStatus();
        initSSE();
    }

    // API Helper with Auth Header
    async function apiFetch(endpoint, options = {}) {
        options.headers = options.headers || {};
        options.headers['Authorization'] = `Bearer ${sessionToken}`;
        const res = await fetch(endpoint, options);
        if (res.status === 401) {
            showLogin();
            throw new Error('Unauthorized');
        }
        return res.json();
    }

    // Fetch Live Dashboard Status Metrics
    async function fetchDashboardStatus() {
        try {
            const data = await apiFetch('/api/status');
            document.getElementById('statGuilds').textContent = data.guildCount || 0;
            document.getElementById('statRam').textContent = `${data.ramMB || 0} MB`;
            document.getElementById('statUptime').textContent = data.uptime || '0h 0m';
            document.getElementById('statCookies').textContent = data.hasCookies ? 'Loaded' : 'Not Found';

            if (data.nowPlaying) {
                document.getElementById('heroTitle').textContent = data.nowPlaying.title;
                document.getElementById('heroAuthor').textContent = data.nowPlaying.author || 'Unknown Artist';
                if (data.nowPlaying.thumbnail) {
                    document.getElementById('heroThumbnail').src = data.nowPlaying.thumbnail;
                }
            }

            if (data.queue && data.queue.length > 0) {
                const queueContainer = document.getElementById('queueItems');
                if (queueContainer) {
                    queueContainer.innerHTML = data.queue.map((track, idx) => `
                        <div class="queue-item" style="display:flex; justify-content:space-between; align-items:center; padding:0.75rem 1rem; margin-bottom:0.5rem; background:rgba(255,255,255,0.04); border-radius:12px; border:1px solid rgba(255,255,255,0.08);">
                            <div>
                                <strong style="color:var(--text-primary); font-size:0.9rem;">${idx + 1}. ${track.title}</strong>
                                <div style="font-size:0.78rem; color:var(--text-secondary);">${track.author || ''} • ${track.duration || ''}</div>
                            </div>
                        </div>
                    `).join('');
                }
            }
        } catch (err) {
            console.error('Failed to fetch dashboard status:', err);
        }
    }

    // Server-Sent Events (SSE) Stream Listener
    function initSSE() {
        if (!window.EventSource) return;
        try {
            const evtSource = new EventSource('/api/events');
            evtSource.onmessage = (event) => {
                try {
                    const data = JSON.parse(event.data);
                    if (data.ramMB) document.getElementById('statRam').textContent = `${data.ramMB} MB`;
                    if (data.uptime) document.getElementById('statUptime').textContent = data.uptime;
                } catch (e) {}
            };
        } catch (e) {
            console.warn('SSE EventSource fallback to HTTP polling');
        }
    }

    // Interactive Player Controls Handlers
    const btnPause = document.getElementById('btnPause');
    const btnSkip = document.getElementById('btnSkip');
    const btnStop = document.getElementById('btnStop');
    const volumeSlider = document.getElementById('volumeSlider');
    const volumeVal = document.getElementById('volumeVal');

    if (btnPause) {
        btnPause.addEventListener('click', async () => {
            await apiFetch('/api/control', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ action: 'pause' })
            });
        });
    }

    if (btnSkip) {
        btnSkip.addEventListener('click', async () => {
            await apiFetch('/api/control', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ action: 'skip' })
            });
        });
    }

    if (btnStop) {
        btnStop.addEventListener('click', async () => {
            await apiFetch('/api/control', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ action: 'stop' })
            });
        });
    }

    if (volumeSlider && volumeVal) {
        volumeSlider.addEventListener('input', (e) => {
            const pct = Math.round(e.target.value * 100);
            volumeVal.textContent = `${pct}%`;
        });
    }

    // Initial Auth Check
    if (sessionToken) {
        showDashboard();
    } else {
        showLogin();
    }
});
