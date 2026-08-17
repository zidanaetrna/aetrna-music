import { useEffect, useState, useRef } from 'react';

const COLOR_MAP: Record<string, string> = {
    'ERROR': '#F87171',
    'WARN':  '#FBBF24',
    'FATAL': '#F43F5E',
    'INFO':  '#34D399',
    'DEBUG': '#60A5FA',
};

const levelColorize = (line: string): string => {
    for (const [level, color] of Object.entries(COLOR_MAP)) {
        if (line.toUpperCase().includes(`[${level}`) || line.toUpperCase().includes(`${level}]`)) {
            return `<span style="color:${color}">${escapeHtml(line)}</span>`;
        }
    }
    return escapeHtml(line);
};

const escapeHtml = (s: string): string => {
    return s.replace(/[&<>"']/g, (c) => ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
    } as Record<string, string>)[c]);
};

export const useLogs = (active: boolean, token?: string | null): { lines: string[]; reconnecting: boolean } => {
    const [lines, setLines] = useState<string[]>([]);
    const [reconnecting, setReconnecting] = useState<boolean>(false);
    const esRef = useRef<EventSource | null>(null);
    const mounted = useRef(false);

    useEffect(() => {
        mounted.current = true;
        if (!active) {
            if (esRef.current) {
                esRef.current.close();
                esRef.current = null;
            }
            return;
        }

        const buildUrl = () => {
            const base = '/api/logs';
            if (token) {
                return `${base}?token=${encodeURIComponent(token)}`;
            }
            return base;
        };

        let retryTimer: ReturnType<typeof setTimeout> | null = null;

        const connect = () => {
            const es = new EventSource(buildUrl());
            esRef.current = es;

            es.onmessage = (event) => {
                if (!mounted.current) return;
                try {
                    const data = JSON.parse(event.data);
                    if (data.type === 'snapshot') {
                        const snap = (data.entries || []).map(levelColorize).reverse();
                        setLines(snap);
                    } else if (data.type === 'line') {
                        const colored = levelColorize(data.entry || '');
                        setLines((prev) => [colored, ...prev].slice(0, 500));
                    }
                    setReconnecting(false);
                } catch (e) {}
            };

            es.onerror = () => {
                if (!mounted.current) return;
                setReconnecting(true);
                try { es.close(); } catch (_) {}
                if (retryTimer) clearTimeout(retryTimer);
                retryTimer = setTimeout(connect, 2000);
            };
        };

        connect();

        return () => {
            mounted.current = false;
            if (retryTimer) clearTimeout(retryTimer);
            if (esRef.current) {
                esRef.current.close();
                esRef.current = null;
            }
        };
    }, [active, token]);

    return { lines, reconnecting };
};
