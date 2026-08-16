/* Aetrna's Music — TypeScript Data Models & Contracts */

export type Language = 'en' | 'id' | 'jp';

export interface GuildOption {
    id: string;
    name: string;
    status: string;
    colorIcon: string;
}

export interface NowPlayingTrack {
    title: string;
    author: string;
    thumbnail: string;
    requested: string;
    duration?: string;
    url?: string;
}

export interface SystemMetrics {
    guildCount: number;
    ramMB: number;
    uptime: string;
    hasCookies: boolean;
    nowPlaying?: NowPlayingTrack;
}

export interface ToastItem {
    id: string;
    message: string;
    type: 'success' | 'error' | 'info';
}
