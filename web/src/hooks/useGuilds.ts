import { useEffect, useState } from 'react';

export interface GuildInfo {
    id: string;
    name: string;
    memberCount: number;
    status: string;
}

export const useGuilds = (token: string | null): { guilds: GuildInfo[]; loading: boolean } => {
    const [guilds, setGuilds] = useState<GuildInfo[]>([]);
    const [loading, setLoading] = useState<boolean>(true);

    useEffect(() => {
        if (token === null) {
            setGuilds([]);
            setLoading(false);
            return;
        }

        let cancelled = false;

        const fetchGuilds = async () => {
            setLoading(true);
            try {
                const response = await fetch('/api/guilds', {
                    headers: {
                        Authorization: `Bearer ${token}`,
                    },
                });

                if (!response.ok) {
                    // 401 or any other error — return empty array
                    if (!cancelled) {
                        setGuilds([]);
                        setLoading(false);
                    }
                    return;
                }

                const data: GuildInfo[] = await response.json();
                if (!cancelled) {
                    setGuilds(data);
                    setLoading(false);
                }
            } catch {
                // Network error — return empty array
                if (!cancelled) {
                    setGuilds([]);
                    setLoading(false);
                }
            }
        };

        fetchGuilds();

        return () => {
            cancelled = true;
        };
    }, [token]);

    return { guilds, loading };
};
