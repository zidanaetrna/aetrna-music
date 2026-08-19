import React, { useState } from 'react';
import { I18nProvider, useI18n } from './context/I18nContext';
import { ToastProvider, useToast } from './context/ToastContext';
import { LoginModal } from './components/LoginModal';
import { Sidebar } from './components/Sidebar';
import { TopBar } from './components/TopBar';
import { OverviewTab } from './components/OverviewTab';
import { PlaylistsModal } from './components/PlaylistsModal';
import { useStatus } from './hooks/useStatus';
import { useLogs } from './hooks/useLogs';
import '../assets/css/style.css';

const DashboardContent: React.FC = () => {
    const [token, setToken] = useState<string | null>(() => localStorage.getItem('aetrna_token'));
    const [activeTab, setActiveTab] = useState<string>('overview');
    const [collapsed, setCollapsed] = useState<boolean>(false);
    const [selectedGuild, setSelectedGuild] = useState<string>('102938475610293847');
    const [searchQuery, setSearchQuery] = useState<string>('');
    const { t } = useI18n();
    const { showToast } = useToast();
    const { status } = useStatus(token);
    const { lines, reconnecting } = useLogs(activeTab === 'logs', token);

    const handleLogout = () => {
        setToken(null);
        localStorage.removeItem('aetrna_token');
        showToast(t('toast_logout'), 'info');
    };

    const handlePlaySearch = async () => {
        const query = searchQuery.trim();
        if (!query) {
            return showToast('Please enter a track name or URL.', 'error');
        }
        try {
            await fetch('/api/control', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + (token ?? '') },
                body: JSON.stringify({ action: 'play', query, guildId: selectedGuild })
            });
        } catch (e) {}
        showToast(t('toast_track_enqueued', { query }), 'success');
        setSearchQuery('');
    };

    const handleKeyDownSearch = (e: React.KeyboardEvent<HTMLInputElement>) => {
        if (e.key === 'Enter') {
            handlePlaySearch();
        }
    };

    if (!token) {
        return <LoginModal onLoginSuccess={(t) => { setToken(t); localStorage.setItem('aetrna_token', t); }} />;
    }

    return (
        <div className="app-container">
            <Sidebar 
                activeTab={activeTab}
                setActiveTab={setActiveTab}
                collapsed={collapsed}
                setCollapsed={setCollapsed}
                onLogout={handleLogout}
            />

            <div className="main-wrapper">
                <TopBar 
                    token={token}
                    activeTab={activeTab}
                    selectedGuild={selectedGuild}
                    setSelectedGuild={setSelectedGuild}
                />

                <main className="main-content">
                    {activeTab === 'overview' && <OverviewTab token={token} status={status} selectedGuild={selectedGuild} />}

                    {activeTab === 'playlists' && <PlaylistsModal token={token} selectedGuild={selectedGuild} />}

                    {activeTab === 'logs' && (
                        <section className="tab-page active">
                            <div className="header-title-row" style={{ marginBottom: '1.5rem' }}>
                                <h2>{t('title_logs')}</h2>
                                <p>{t('desc_logs')}</p>
                            </div>
                            <div className="glass-card" style={{ padding: '1.5rem' }}>
                                <div style={{ fontFamily: 'monospace', fontSize: '0.8rem', background: '#000', color: '#10B981', padding: '1.25rem', borderRadius: '10px', height: '350px', overflowY: 'auto', lineHeight: '1.6' }}>
                                    {reconnecting && (
                                        <div style={{ color: '#F59E0B', marginBottom: '0.5rem' }}>[WARN] Reconnecting to log stream...</div>
                                    )}
                                    {lines.length === 0 && !reconnecting && (
                                        <div style={{ color: '#6B7280' }}>Waiting for log events...</div>
                                    )}
                                    {lines.map((line, i) => (
                                        <div key={i} dangerouslySetInnerHTML={{ __html: line }} />
                                    ))}
                                </div>
                            </div>
                        </section>
                    )}

                    {activeTab === 'queue' && (
                        <section className="tab-page active">
                            <div className="header-title-row" style={{ marginBottom: '1.5rem' }}>
                                <h2>{t('title_player')}</h2>
                                <p>{t('desc_player')}</p>
                            </div>
                            <div className="glass-card" style={{ padding: '1.5rem' }}>
                                <label style={{ fontSize: '0.82rem', fontWeight: 700, color: 'var(--cf-text-subtle)', display: 'block', marginBottom: '0.6rem' }}>
                                    {t('lbl_search')}
                                </label>
                                <div style={{ display: 'flex', gap: '0.75rem' }}>
                                    <input 
                                        type="text" 
                                        value={searchQuery}
                                        onChange={(e) => setSearchQuery(e.target.value)}
                                        onKeyDown={handleKeyDownSearch}
                                        placeholder="Paste YouTube / Spotify URL or track title..." 
                                        style={{ flex: 1, padding: '0.75rem 1rem', background: '#14151B', border: '1px solid var(--cf-card-border)', borderRadius: '6px', color: '#FFF', outline: 'none' }} 
                                    />
                                    <button onClick={handlePlaySearch} className="btn-ctrl">{t('btn_playtrack')}</button>
                                </div>
                            </div>
                        </section>
                    )}

                    {activeTab === 'dsp' && (
                        <section className="tab-page active">
                            <div className="header-title-row" style={{ marginBottom: '1.5rem' }}>
                                <h2>{t('title_dsp')}</h2>
                                <p>{t('desc_dsp')}</p>
                            </div>
                            <div className="glass-card" style={{ padding: '1.5rem' }}>
                                <label style={{ fontSize: '0.82rem', fontWeight: 700, color: 'var(--cf-text-subtle)', display: 'block', marginBottom: '0.85rem' }}>
                                    {t('lbl_dsp')}
                                </label>
                                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.75rem' }}>
                                    <button className="btn-ctrl dsp-pill">Bass Boost</button>
                                    <button className="btn-ctrl dsp-pill">Nightcore Speed</button>
                                    <button className="btn-ctrl dsp-pill">8D Spatial Panning</button>
                                </div>
                            </div>
                        </section>
                    )}

                    {activeTab === 'favorites' && (
                        <section className="tab-page active">
                            <div className="header-title-row" style={{ marginBottom: '1.5rem' }}>
                                <h2>{t('title_favorites')}</h2>
                                <p>{t('desc_favorites')}</p>
                            </div>
                            <div className="glass-card" style={{ padding: '1.5rem' }}>
                                <h3>{t('saved_favs')}</h3>
                                <p style={{ color: 'var(--cf-text-muted)', marginTop: '0.5rem' }}>No saved favorites yet.</p>
                            </div>
                        </section>
                    )}

                    {activeTab === 'settings' && (
                        <section className="tab-page active">
                            <div className="header-title-row" style={{ marginBottom: '1.5rem' }}>
                                <h2>{t('title_settings')}</h2>
                                <p>{t('desc_settings')}</p>
                            </div>
                            <div className="glass-card" style={{ padding: '1.5rem' }}>
                                <h3>Platform Configuration</h3>
                                <div style={{ display: 'flex', justifyContent: 'space-between', padding: '0.85rem 0', borderBottom: '1px solid var(--cf-card-border)' }}>
                                    <span>YouTube Cookies:</span>
                                    {status?.hasCookies === true
                                        ? <span className="stat-badge green">cookies.txt Loaded</span>
                                        : <span className="stat-badge grey">Not Loaded</span>
                                    }
                                </div>
                                {status?.clientIP && (
                                    <div style={{ display: 'flex', justifyContent: 'space-between', padding: '0.85rem 0', borderBottom: '1px solid var(--cf-card-border)' }}>
                                        <span>Reverse Proxy:</span>
                                        <span className="stat-badge">Client IP: {status.clientIP}</span>
                                    </div>
                                )}
                            </div>
                        </section>
                    )}
                </main>
            </div>
        </div>
    );
};

export const App: React.FC = () => {
    return (
        <I18nProvider>
            <ToastProvider>
                <DashboardContent />
            </ToastProvider>
        </I18nProvider>
    );
};

export default App;
