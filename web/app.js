/* Aetrna's Music Dashboard — Single Page Application Logic */

document.addEventListener('DOMContentLoaded', () => {
    const loginModal = document.getElementById('loginModal');
    const loginForm = document.getElementById('loginForm');
    const passwordInput = document.getElementById('passwordInput');
    const togglePasswordBtn = document.getElementById('togglePasswordBtn');
    const loginErrorText = document.getElementById('loginErrorText');
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
    loginForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        loginError.classList.add('hidden');
        const password = passwordInput.value;

        try {
            const res = await fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password })
            });

            if (!res.ok) {
                loginErrorText.textContent = 'Invalid password. Please check your .env configuration.';
                loginError.classList.remove('hidden');
                return;
            }

            const data = await res.json();
            sessionToken = data.token;
            localStorage.setItem('aetrna_token', sessionToken);
            showDashboard();
        } catch (err) {
            loginErrorText.textContent = 'Connection error to Aetrna Bot API server.';
            loginError.classList.remove('hidden');
        }
    });

    // Logout Handler
    logoutBtn.addEventListener('click', () => {
        sessionToken = '';
        localStorage.removeItem('aetrna_token');
        showLogin();
    });

    function showLogin() {
        loginModal.classList.remove('hidden');
        dashboardApp.classList.add('hidden');
    }

    function showDashboard() {
        loginModal.classList.add('hidden');
        dashboardApp.classList.remove('hidden');
        fetchDashboardStatus();
        startPolling();
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

    // Fetch System Status Metrics
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
        } catch (err) {
            console.error('Failed to fetch dashboard status:', err);
        }
    }

    function startPolling() {
        setInterval(() => {
            if (!dashboardApp.classList.contains('hidden')) {
                fetchDashboardStatus();
            }
        }, 5000);
    }

    // Initial Auth Check
    if (sessionToken) {
        showDashboard();
    } else {
        showLogin();
    }
});
