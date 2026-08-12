const dgram = require('dgram');
const dns = require('dns');

const host = 'c-hkg09-904a5106.discord.media';
const port = 2096;

console.log(`🔍 Resolving ${host}...`);

dns.resolve4(host, (err, addresses) => {
    if (err) {
        console.error('❌ IPv4 DNS resolution failed:', err);
        return;
    }
    console.log(`✅ IPv4 addresses for ${host}:`, addresses);

    const targetIp = addresses[0];
    console.log(`📡 Testing UDP ping to ${targetIp}:${port}...`);

    const socket = dgram.createSocket('udp4');
    
    // 74-byte Discord UDP IP Discovery request packet format
    const packet = Buffer.alloc(74);
    packet.writeUInt16BE(0x0001, 0); // Type: Request
    packet.writeUInt16BE(70, 2);     // Length: 70
    packet.writeUInt32BE(1, 4);      // SSRC: 1

    let received = false;

    socket.on('message', (msg, rinfo) => {
        received = true;
        console.log(`🎉 UDP RESPONSE RECEIVED from ${rinfo.address}:${rinfo.port}! Packet length: ${msg.length}`);
        socket.close();
    });

    socket.on('error', (err) => {
        console.error('❌ UDP Socket Error:', err.message);
    });

    socket.send(packet, 0, packet.length, port, targetIp, (err) => {
        if (err) console.error('❌ Send error:', err);
        else console.log(`📤 UDP packet sent to ${targetIp}:${port}. Waiting 3s for reply...`);
    });

    setTimeout(() => {
        if (!received) {
            console.error(`💥 UDP TIMEOUT: No response from ${targetIp}:${port} after 3 seconds! UDP is BLOCKED on this VPS!`);
            socket.close();
        }
    }, 3000);
});
