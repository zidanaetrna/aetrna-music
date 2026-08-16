import React from 'react';
import { useI18n } from '../context/I18nContext';
import { useToast } from '../context/ToastContext';

interface TopBarProps {
    activeTab: string;
    selectedGuild: string;
    setSelectedGuild: (guildId: string) => void;
}

export const TopBar: React.FC<TopBarProps> = ({
    activeTab,
    selectedGuild,
    setSelectedGuild
}) => {
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
                    <option value="102938475610293847">🟢 Aetrna Lounge (Playing: Echoes of Eternity)</option>
                    <option value="293847102938471029">🔵 Chill Vibes & Gaming (Queued: 3 tracks)</option>
                    <option value="384729103847291038">🟣 Code & Music Squad (Idle)</option>
                    <option value="482910384729103847">⚪ Anime Music Hub (Idle)</option>
                </select>
            </div>
        </header>
    );
};
