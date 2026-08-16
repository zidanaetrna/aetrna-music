import React, { createContext, useContext, useState, useEffect } from 'react';
import { Language } from '../types';

interface I18nContextType {
    language: Language;
    setLanguage: (lang: Language) => void;
    t: (key: string, params?: Record<string, string>) => string;
}

const translations: Record<Language, Record<string, string>> = {
    en: {
        welcome_back: "Welcome Back!",
        sign_in_desc: "Sign in to access your dashboard and continue optimizing your music playback.",
        pass_label: "Dashboard Password",
        btn_signin: "Sign In",
        nav_observe: "Observe",
        nav_overview: "Overview",
        nav_logs: "System Logs",
        nav_playback: "Playback",
        nav_queue: "Queue & Player",
        nav_dsp: "DSP Equalizer",
        nav_favorites: "Favorites",
        nav_system: "System",
        nav_settings: "System Information",
        btn_logout: "Logout",
        target_guild: "Target Guild:",
        title_overview: "Analytics Overview",
        desc_overview: "Real-time audio processing metrics, active guilds, and cache telemetry",
        stat_guilds: "ACTIVE DISCORD GUILDS",
        sub_guilds: "Connected Servers",
        stat_ram: "MEMORY CONSUMPTION",
        sub_ram: "Heap Alloc",
        stat_uptime: "SYSTEM UPTIME",
        sub_uptime: "No Interruption",
        btn_pause: "Pause",
        btn_skip: "Skip",
        btn_stop: "Stop",
        btn_kick: "Kick Bot (Target Guild)",
        vol_label: "Volume:",
        title_cache: "CACHE MEMORY TELEMETRY",
        title_queue: "ACTIVE QUEUED TRACKS",
        th_title: "Track Title",
        th_source: "Source Protocol",
        th_duration: "Duration",
        th_requested: "Requested By",
        th_status: "Status",
        title_logs: "System Logs & Telemetry",
        desc_logs: "Live stdout/stderr stream telemetry from Go microservice and Node voice bridge",
        title_player: "Queue & Live Player",
        desc_player: "Search tracks, manage live playback, and control server queues",
        lbl_search: "QUICK TRACK SEARCH & PLAY",
        btn_playtrack: "Play Track",
        title_dsp: "Audio DSP Equalizer",
        desc_dsp: "Apply real-time FFmpeg audio filters and DSP effects",
        lbl_dsp: "AUDIO DSP EQUALIZER FILTERS",
        title_favorites: "Favorites & Saved Playlists",
        desc_favorites: "Access your saved tracks and recent playback logs",
        saved_favs: "Saved Favorites",
        recent_history: "Recent History",
        title_settings: "System Information",
        desc_settings: "Platform parameters, YouTube cookies, reverse proxy, and database maintenance",
        toast_lang_switched: "Switched language to English (US)",
        toast_wallet_copied: "Wallet address copied to clipboard!",
        toast_guild_switched: "Switched active guild target to: {guild}",
        toast_track_enqueued: "Enqueued track \"{query}\"",
        toast_filter_applied: "Applied Audio DSP Filter: {filter}",
        toast_auth_success: "Authentication successful. Welcome back!",
        toast_logout: "Logged out of dashboard session.",
        toast_disconnected: "Disconnected bot in Target Guild ({guildId})",
        toast_paused: "Playback paused",
        toast_skipped: "Skipped to next track",
        toast_stopped: "Playback stopped and queue cleared"
    },
    id: {
        welcome_back: "Welcome Back!",
        sign_in_desc: "Masuk ke dashboard untuk kelola & optimalkan pemutaran musik Discord.",
        pass_label: "Password Dashboard",
        btn_signin: "Sign In",
        nav_observe: "Observe",
        nav_overview: "Overview",
        nav_logs: "System Logs",
        nav_playback: "Playback",
        nav_queue: "Queue & Player",
        nav_dsp: "DSP Equalizer",
        nav_favorites: "Favorites",
        nav_system: "System",
        nav_settings: "System Information",
        btn_logout: "Logout",
        target_guild: "Target Server:",
        title_overview: "Analytics Overview",
        desc_overview: "Metrik pemrosesan audio real-time, server aktif, dan telemetri cache",
        stat_guilds: "ACTIVE DISCORD GUILDS",
        sub_guilds: "Server Terhubung",
        stat_ram: "MEMORY CONSUMPTION",
        sub_ram: "Alokasi Memory",
        stat_uptime: "SYSTEM UPTIME",
        sub_uptime: "Aktif Tanpa Putus",
        btn_pause: "Pause",
        btn_skip: "Skip",
        btn_stop: "Stop",
        btn_kick: "Kick Bot (Target Server)",
        vol_label: "Volume:",
        title_cache: "CACHE MEMORY TELEMETRY",
        title_queue: "ACTIVE QUEUED TRACKS",
        th_title: "Judul Lagu",
        th_source: "Sumber Stream",
        th_duration: "Durasi",
        th_requested: "Request Oleh",
        th_status: "Status",
        title_logs: "System Logs & Telemetry",
        desc_logs: "Stream telemetry stdout/stderr langsung dari microservice Go dan Node voice bridge",
        title_player: "Queue & Live Player",
        desc_player: "Cari lagu, kelola pemutaran langsung, dan kontrol queue server",
        lbl_search: "QUICK SEARCH & PLAY TRACK",
        btn_playtrack: "Play Track",
        title_dsp: "Audio DSP Equalizer",
        desc_dsp: "Terapkan filter audio FFmpeg dan efek DSP secara real-time",
        lbl_dsp: "AUDIO DSP EQUALIZER FILTERS",
        title_favorites: "Favorites & Playlists",
        desc_favorites: "Akses lagu favorit dan riwayat pemutaran terakhir",
        saved_favs: "Lagu Favorit",
        recent_history: "Riwayat Terakhir",
        title_settings: "System Information",
        desc_settings: "Parameter platform, cookies YouTube, reverse proxy, dan maintenance database",
        toast_lang_switched: "Ganti bahasa ke Bahasa Indonesia",
        toast_wallet_copied: "Alamat wallet berhasil di-copy ke clipboard!",
        toast_guild_switched: "Pindah target server ke: {guild}",
        toast_track_enqueued: "Lagu \"{query}\" berhasil masuk queue",
        toast_filter_applied: "Pasang Filter DSP: {filter}",
        toast_auth_success: "Login berhasil. Welcome back!",
        toast_logout: "Berhasil logout dari session dashboard.",
        toast_disconnected: "Bot di-kick & disconnect dari voice di Target Server ({guildId})",
        toast_paused: "Pemutaran lagu di-pause",
        toast_skipped: "Lagu di-skip ke berikutnya",
        toast_stopped: "Pemutaran di-stop & queue dibersihkan"
    },
    jp: {
        welcome_back: "おかえりなさい！",
        sign_in_desc: "ダッシュボードにサインインして、Discordの音楽再生を管理します。",
        pass_label: "ダッシュボードパスワード",
        btn_signin: "サインイン",
        nav_observe: "監視",
        nav_overview: "概要",
        nav_logs: "システムログ",
        nav_playback: "再生管理",
        nav_queue: "キューとプレイヤー",
        nav_dsp: "DSPイコライザー",
        nav_favorites: "お気に入り",
        nav_system: "システム",
        nav_settings: "システム設定",
        btn_logout: "ログアウト",
        target_guild: "対象サーバー:",
        title_overview: "アナリティクス概要",
        desc_overview: "リアルタイムオーディオ処理メトリクス、アクティブサーバー、キャッシュテレメトリ",
        stat_guilds: "アクティブなDiscordサーバー",
        sub_guilds: "接続中サーバー",
        stat_ram: "メモリ使用量",
        sub_ram: "ヒープ割り当て",
        stat_uptime: "システム稼働時間",
        sub_uptime: "無中断稼働",
        btn_pause: "一時停止",
        btn_skip: "スキップ",
        btn_stop: "停止",
        btn_kick: "ボットをキック",
        vol_label: "音量:",
        title_cache: "キャッシュメモリテレメトリ",
        title_queue: "再生キューリスト",
        th_title: "曲名",
        th_source: "音源プロトコル",
        th_duration: "再生時間",
        th_requested: "リクエスト者",
        th_status: "ステータス",
        title_logs: "システムログとテレメトリ",
        desc_logs: "GoマイクロサービスとNodeボイスブリッジのリアルタイムログストリーム",
        title_player: "キューとライブプレイヤー",
        desc_player: "曲の検索、ライブ再生の管理、サーバーキューの操作",
        lbl_search: "クイック曲検索と再生",
        btn_playtrack: "再生",
        title_dsp: "オーディオDSPイコライザー",
        desc_dsp: "FFmpegオーディオフィルターとDSPエフェクトを適用",
        lbl_dsp: "オーディオDSPイコライザーフィルター",
        title_favorites: "お気に入りと履歴",
        desc_favorites: "保存された曲と最近の再生履歴にアクセス",
        saved_favs: "お気に入りの曲",
        recent_history: "最近の履歴",
        title_settings: "システム設定",
        desc_settings: "プラットフォームパラメータ、YouTubeクッキー、リバースプロキシ、DBメンテナンス",
        toast_lang_switched: "言語を日本語に切り替えました",
        toast_wallet_copied: "ウォレットアドレスをクリップボードにコピーしました！",
        toast_guild_switched: "対象サーバーを切り替えました: {guild}",
        toast_track_enqueued: "曲 \"{query}\" をキューに追加しました",
        toast_filter_applied: "DSPフィルターを適用しました: {filter}",
        toast_auth_success: "認証に成功しました。おかえりなさい！",
        toast_logout: "ログアウトしました。",
        toast_disconnected: "対象サーバー ({guildId}) のボットを切断しました",
        toast_paused: "再生を一時停止しました",
        toast_skipped: "次の曲にスキップしました",
        toast_stopped: "再生を停止しキューを消去しました"
    }
};

const I18nContext = createContext<I18nContextType | undefined>(undefined);

export const I18nProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [language, setLanguageState] = useState<Language>(() => {
        return (localStorage.getItem('aetrna_lang') as Language) || 'en';
    });

    const setLanguage = (lang: Language) => {
        setLanguageState(lang);
        localStorage.setItem('aetrna_lang', lang);
    };

    const t = (key: string, params: Record<string, string> = {}): string => {
        const langDict = translations[language] || translations.en;
        let val = langDict[key] || translations.en[key] || key;
        Object.keys(params).forEach(pKey => {
            val = val.replace(`{${pKey}}`, params[pKey]);
        });
        return val;
    };

    return (
        <I18nContext.Provider value={{ language, setLanguage, t }}>
            {children}
        </I18nContext.Provider>
    );
};

export const useI18n = () => {
    const ctx = useContext(I18nContext);
    if (!ctx) throw new Error('useI18n must be used within I18nProvider');
    return ctx;
};
