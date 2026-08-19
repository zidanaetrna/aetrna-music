import { useEffect, useState } from 'react';

export interface StatusData {
  guildCount: number;
  ramMB: number;
  uptime: string;
  hasCookies: boolean;
  nowPlaying: {
    title: string;
    author: string;
    duration: string;
    thumbnail: string;
    requested: string;
  } | null;
  queue: Array<{
    title: string;
    author: string;
    duration: string;
    requested: string;
  }>;
  clientIP: string;
}

export const useStatus = (token: string | null, selectedGuild?: string): {
  status: StatusData | null;
  loading: boolean;
  error: string | null;
} => {
  const [status, setStatus] = useState<StatusData | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (token === null) {
      setLoading(false);
      return;
    }

    const fetchStatus = async () => {
      try {
        const query = selectedGuild ? `?guildId=${encodeURIComponent(selectedGuild)}` : '';
        const response = await fetch(`/api/status${query}`, {
          headers: {
            Authorization: `Bearer ${token}`,
          },
        });

        if (!response.ok) {
          const text = await response.text().catch(() => response.statusText);
          setError(text || `Request failed with status ${response.status}`);
          return;
        }

        const data: StatusData = await response.json();
        setStatus(data);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch status');
      } finally {
        setLoading(false);
      }
    };

    fetchStatus();

    const intervalId = setInterval(fetchStatus, 10_000);

    return () => {
      clearInterval(intervalId);
    };
  }, [token]);

  return { status, loading, error };
};
