const https = require('https');
const tls = require('tls');

const host = 'c-hkg09-904a5106.discord.media';
const port = 2096;

console.log(`🔍 Testing HTTPS/TLS connection to https://${host}:${port}...`);

const req = https.get(`https://${host}:${port}`, { timeout: 5000 }, (res) => {
    console.log(`✅ HTTPS response status: ${res.statusCode} ${res.statusMessage}`);
    res.resume();
});

req.on('timeout', () => {
    console.error(`💥 TIMEOUT connecting to https://${host}:${port}`);
    req.destroy();
});

req.on('error', (err) => {
    console.error(`❌ TLS/HTTPS Error to ${host}:${port} —`, err.code || err.message);
});
