import { useEffect, useState } from 'react';

export interface TelemetryData {
    activeGuilds?: number;
    memoryUsage?: string;
    uptime?: string;
    timestamp?: number;
}

export const useWebSocket = () => {
    const [connected, setConnected] = useState<boolean>(false);
    const [telemetry, setTelemetry] = useState<TelemetryData>({});

    useEffect(() => {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/api/ws`;
        let socket: WebSocket | null = null;
        let retryTimer: ReturnType<typeof setTimeout> | undefined;

        const connect = () => {
            try {
                socket = new WebSocket(wsUrl);

                socket.onopen = () => {
                    setConnected(true);
                };

                socket.onmessage = (event) => {
                    try {
                        const parsed = JSON.parse(event.data);
                        if (parsed.type === 'telemetry') {
                            setTelemetry(parsed.data);
                        }
                    } catch (e) {}
                };

                socket.onclose = () => {
                    setConnected(false);
                    retryTimer = setTimeout(connect, 3000);
                };

                socket.onerror = () => {
                    setConnected(false);
                    if (socket) socket.close();
                };
            } catch (err) {
                setConnected(false);
                retryTimer = setTimeout(connect, 3000);
            }
        };

        connect();

        return () => {
            if (socket) socket.close();
            if (retryTimer) clearTimeout(retryTimer);
        };
    }, []);

    return { connected, telemetry };
};
