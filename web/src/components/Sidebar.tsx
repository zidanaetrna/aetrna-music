import React, { useState, useEffect, useRef } from 'react';
import { useI18n } from '../context/I18nContext';
import { useToast } from '../context/ToastContext';
import { Language } from '../types';

interface SidebarProps {
    activeTab: string;
    setActiveTab: (tab: string) => void;
    collapsed: boolean;
    setCollapsed: (collapsed: boolean) => void;
    onLogout: () => void;
}

export const Sidebar: React.FC<SidebarProps> = ({
    activeTab,
    setActiveTab,
    collapsed,
    setCollapsed,
    onLogout
}) => {
    const { language, setLanguage, t } = useI18n();
    const { showToast } = useToast();
    const [searchFilter, setSearchFilter] = useState('');
    const searchInputRef = useRef<HTMLInputElement>(null);

    // Ctrl + K Keyboard Shortcut Listener
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
                e.preventDefault();
                if (searchInputRef.current) {
                    searchInputRef.current.focus();
                }
            }
        };
        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, []);

    const handleLanguageChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
        const newLang = e.target.value as Language;
        setLanguage(newLang);
        showToast(t('toast_lang_switched'), 'success');
    };

    const matchesSearch = (text: string) => {
        if (!searchFilter.trim()) return true;
        return text.toLowerCase().includes(searchFilter.toLowerCase());
    };

    return (
        <aside className={`sidebar ${collapsed ? 'collapsed' : ''}`}>
            <div>
                <div className="cf-top-account-selector">
                    <div className="cf-account-brand">
                        <img src="artwork.png" alt="Aetrna Logo" className="row-thumb" style={{ width: 26, height: 26, borderRadius: '50%' }} />
                        <span className="cf-account-name sidebar-label">Aetrna's Music</span>
                    </div>
                    <button onClick={() => setCollapsed(!collapsed)} className="cf-collapse-btn" title="Toggle Sidebar Expand/Collapse">
                        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2"><polyline points="15 18 9 12 15 6"/></svg>
                    </button>
                </div>

                {/* Quick Search Input Box with Ctrl K Focus */}
                <div className="cf-search-box">
                    <span className="cf-search-icon">
                        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
                    </span>
                    <input 
                        ref={searchInputRef}
                        type="text" 
                        value={searchFilter}
                        onChange={(e) => setSearchFilter(e.target.value)}
                        placeholder="Quick search..." 
                        className="cf-search-input sidebar-label" 
                    />
                    <span className="cf-search-kbd sidebar-label">Ctrl K</span>
                </div>

                {/* OBSERVE CATEGORY */}
                <div className="cf-nav-section-title sidebar-label">{t('nav_observe')}</div>
                <nav className="nav-menu">
                    {matchesSearch(t('nav_overview')) && (
                        <button className={`nav-item ${activeTab === 'overview' ? 'active' : ''}`} onClick={() => setActiveTab('overview')}>
                            <div className="nav-item-left">
                                <span className="nav-icon">
                                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>
                                </span>
                                <span className="sidebar-label">{t('nav_overview')}</span>
                            </div>
                        </button>
                    )}
                    {matchesSearch(t('nav_logs')) && (
                        <button className={`nav-item ${activeTab === 'logs' ? 'active' : ''}`} onClick={() => setActiveTab('logs')}>
                            <div className="nav-item-left">
                                <span className="nav-icon">
                                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
                                </span>
                                <span className="sidebar-label">{t('nav_logs')}</span>
                            </div>
                        </button>
                    )}
                </nav>

                {/* PLAYBACK CATEGORY */}
                <div className="cf-nav-section-title sidebar-label">{t('nav_playback')}</div>
                <nav className="nav-menu">
                    {matchesSearch(t('nav_queue')) && (
                        <button className={`nav-item ${activeTab === 'queue' ? 'active' : ''}`} onClick={() => setActiveTab('queue')}>
                            <div className="nav-item-left">
                                <span className="nav-icon">
                                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2"><path d="M9 18V5l12-2v13M9 9l12-2M9 18a3 3 0 11-6 0 3 3 0 016 0zM21 16a3 3 0 11-6 0 3 3 0 016 0z"/></svg>
                                </span>
                                <span className="sidebar-label">{t('nav_queue')}</span>
                            </div>
                        </button>
                    )}
                    {matchesSearch(t('nav_dsp')) && (
                        <button className={`nav-item ${activeTab === 'dsp' ? 'active' : ''}`} onClick={() => setActiveTab('dsp')}>
                            <div className="nav-item-left">
                                <span className="nav-icon">
                                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>
                                </span>
                                <span className="sidebar-label">{t('nav_dsp')}</span>
                            </div>
                        </button>
                    )}
                    {matchesSearch('Playlists') && (
                        <button className={`nav-item ${activeTab === 'playlists' ? 'active' : ''}`} onClick={() => setActiveTab('playlists')}>
                            <div className="nav-item-left">
                                <span className="nav-icon">
                                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2"><path d="M19 21l-7-5-7 5V5a2 2 0 012-2h10a2 2 0 012 2z"/></svg>
                                </span>
                                <span className="sidebar-label">Playlists</span>
                            </div>
                        </button>
                    )}
                    {matchesSearch(t('nav_favorites')) && (
                        <button className={`nav-item ${activeTab === 'favorites' ? 'active' : ''}`} onClick={() => setActiveTab('favorites')}>
                            <div className="nav-item-left">
                                <span className="nav-icon">
                                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
                                </span>
                                <span className="sidebar-label">{t('nav_favorites')}</span>
                            </div>
                        </button>
                    )}
                </nav>

                {/* SYSTEM CATEGORY */}
                <div className="cf-nav-section-title sidebar-label">{t('nav_system')}</div>
                <nav className="nav-menu">
                    {matchesSearch(t('nav_settings')) && (
                        <button className={`nav-item ${activeTab === 'settings' ? 'active' : ''}`} onClick={() => setActiveTab('settings')}>
                            <div className="nav-item-left">
                                <span className="nav-icon">
                                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-2 2 2 2 0 01-2-2v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83 0 2 2 0 010-2.83l.06-.06a1.65 1.65 0 00.33-1.82 1.65 1.65 0 00-1.51-1H3a2 2 0 01-2-2 2 2 0 012-2h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 010-2.83 2 2 0 012.83 0l.06.06a1.65 1.65 0 001.82.33H9a1.65 1.65 0 001-1.51V3a2 2 0 012-2 2 2 0 012 2v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 0 2 2 0 010 2.83l-.06.06a1.65 1.65 0 00-.33 1.82V9a1.65 1.65 0 001.51 1H21a2 2 0 012 2 2 2 0 01-2 2h-.09a1.65 1.65 0 00-1.51 1z"/></svg>
                                </span>
                                <span className="sidebar-label">{t('nav_settings')}</span>
                            </div>
                        </button>
                    )}
                </nav>

                <div className="language-selector-wrapper sidebar-label">
                    <select value={language} onChange={handleLanguageChange} className="lang-select">
                        <option value="en">English (US)</option>
                        <option value="id">Bahasa Indonesia</option>
                        <option value="jp">日本語 (Japanese)</option>
                    </select>
                </div>
            </div>

            <div className="user-footer">
                <button onClick={onLogout} className="btn-ghost" style={{ width: '100%', marginBottom: '0.4rem' }}>
                    {t('btn_logout')}
                </button>
                <div className="sidebar-label" style={{ fontSize: '0.72rem', color: 'var(--cf-text-muted)', textAlign: 'center' }}>
                    Aetrna's Music v2.1.8
                </div>
            </div>
        </aside>
    );
};
