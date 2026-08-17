import React from 'react';
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

    const handleGuildChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
        const val = e.target.value;
        setSelectedGuild(val);
        const name = e.target.options[e.target.selectedIndex].text.split('(')[0].trim();
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
                    {loading && <option value="" disabled>Loading guilds…</option>}
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
