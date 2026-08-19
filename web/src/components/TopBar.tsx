import React, { useEffect } from 'react';
import { useI18n } from '../context/I18nContext';
import { useToast } from '../context/ToastContext';
import { useGuilds, GuildInfo } from '../hooks/useGuilds';

interface TopBarProps {
    activeTab: string;
    selectedGuild: string;
    setSelectedGuild: (guildId: string) => void;
    token?: string | null;
}

export const TopBar: React.FC<TopBarProps> = ({
    activeTab,
    selectedGuild,
    setSelectedGuild,
    token = null
}) => {
    const { guilds, loading } = useGuilds(token ?? null);
    const { t } = useI18n();
    const { showToast } = useToast();

    useEffect(() => {
        if (guilds.length > 0) {
            const playingGuild = guilds.find(g => g.status === 'playing');
            if (playingGuild && (!selectedGuild || !guilds.some(g => g.id === selectedGuild))) {
                setSelectedGuild(playingGuild.id);
            } else if (!selectedGuild || !guilds.some(g => g.id === selectedGuild)) {
                setSelectedGuild(guilds[0].id);
            }
        }
    }, [guilds, selectedGuild, setSelectedGuild]);

    const handleGuildChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
        const val = e.target.value;
        setSelectedGuild(val);
        const opt = e.target.options && e.target.selectedIndex >= 0 ? e.target.options[e.target.selectedIndex] : null;
        const name = opt?.text ? opt.text.split('(')[0].trim() : val;
        showToast(t('toast_guild_switched', { guild: name }), 'info');
    };

    return (
        <header className="cf-topbar">
            <div className="cf-breadcrumb">
                <span>Account</span>
                <span>/</span>
                <strong>aetrna.music</strong>
                <span>/</span>
                <span style={{ textTransform: 'capitalize' }}>{activeTab}</span>
            </div>

            <div className="cf-guild-selector-bar">
                <label htmlFor="guildSelect" style={{ fontSize: '0.78rem', fontWeight: 700, color: 'var(--cf-text-subtle)' }}>
                    {t('target_guild')}
                </label>
                <select 
                    id="guildSelect" 
                    value={selectedGuild} 
                    onChange={handleGuildChange}
                    className="cf-guild-dropdown"
                >
                    {loading && guilds.length === 0 && <option value="" disabled>Loading guilds…</option>}
                    {guilds.length === 0 && !loading && <option value="" disabled>No active Discord servers</option>}
                    {guilds.map((g: GuildInfo) => (
                        <option key={g.id} value={g.id}>
                            {g.status === 'playing' ? '🟢' : '⚪'} {g.name} ({g.status})
                        </option>
                    ))}
                </select>
            </div>
        </header>
    );
};
