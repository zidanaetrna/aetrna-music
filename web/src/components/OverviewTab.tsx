import React, { useState } from 'react';
import { useI18n } from '../context/I18nContext';
import { useToast } from '../context/ToastContext';
import { useWebSocket } from '../hooks/useWebSocket';
import { StatusData } from '../hooks/useStatus';

export interface OverviewTabProps {
    selectedGuild: string;
    token?: string | null;
    status?: StatusData | null;
}

export const OverviewTab: React.FC<OverviewTabProps> = ({ selectedGuild, token = null, status = null }) => {
    const { t } = useI18n();
    const { showToast } = useToast();
    const { connected, telemetry } = useWebSocket();
    const [volume, setVolume] = useState(1.0);

    const handleAction = async (action: string, toastKey: string, type: 'info' | 'error' | 'success' = 'info') => {
        try {
            await fetch('/api/control', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': 'Bearer ' + (token ?? ''),
                },
                body: JSON.stringify({ action, guildId: selectedGuild })
            });
        } catch (e) {}
        showToast(t(toastKey, { guildId: selectedGuild }), type);
    };

    return (
        <section className="tab-page active">
            <div className="header-title-row" style={{ marginBottom: '1.5rem', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div className="header-title">
                    <h2>{t('title_overview')}</h2>
                    <p>{t('desc_overview')}</p>
                </div>
                <div className="telemetry-ws-badge" style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', fontSize: '0.78rem', fontWeight: 700, padding: '0.35rem 0.75rem', borderRadius: '20px', background: connected ? 'rgba(16, 185, 129, 0.15)' : 'rgba(234, 179, 8, 0.15)', color: connected ? '#10B981' : '#EAB308', border: `1px solid ${connected ? 'rgba(16, 185, 129, 0.3)' : 'rgba(234, 179, 8, 0.3)'}` }}>
                    <svg viewBox="0 0 24 24" width="10" height="10" fill="currentColor"><circle cx="12" cy="12" r="6"/></svg>
                    <span>{connected ? 'WebSocket Stream Active' : 'Polling Sync Mode'}</span>
                </div>
            </div>

            {/* Metrics Grid */}
            <div className="stats-grid">
                <div className="glass-card stat-card">
                    <div className="stat-header">
                        <span className="stat-title">{t('stat_guilds')}</span>
                        <span className="stat-badge green">Live Sync</span>
                    </div>
                    <div className="stat-value-row">
                        <span className="stat-value">{telemetry.activeGuilds ?? '—'}</span>
                        <span style={{ color: 'var(--cf-text-muted)', fontSize: '0.75rem' }}>{t('sub_guilds')}</span>
                    </div>
                </div>

                <div className="glass-card stat-card">
                    <div className="stat-header">
                        <span className="stat-title">{t('stat_ram')}</span>
                        <span className="stat-badge blue">Sub-ms</span>
                    </div>
                    <div className="stat-value-row">
                        <span className="stat-value">{telemetry.memoryUsage ?? '—'}</span>
                        <span style={{ color: 'var(--cf-text-muted)', fontSize: '0.75rem' }}>{t('sub_ram')}</span>
                    </div>
                </div>

                <div className="glass-card stat-card">
                    <div className="stat-header">
                        <span className="stat-title">{t('stat_uptime')}</span>
                        <span className="stat-badge green">100%</span>
                    </div>
                    <div className="stat-value-row">
                        <span className="stat-value">{status?.uptime ?? '—'}</span>
                        <span style={{ color: 'var(--cf-text-muted)', fontSize: '0.75rem' }}>{t('sub_uptime')}</span>
                    </div>
                </div>
            </div>

            {/* Middle Grid Stage */}
            <div className="middle-grid">
                {/* Left: Now Playing & Controls */}
                <div className="glass-card now-playing-card">
                    <div className="now-playing-header">
                        <span className="live-badge">
                            <svg viewBox="0 0 24 24" width="12" height="12" fill="currentColor"><circle cx="12" cy="12" r="6"/></svg>
                            Live Stream Engine
                        </span>
                        {status?.hasCookies === true
                            ? <span className="stat-badge green">YouTube Cookies Loaded</span>
                            : <span className="stat-badge">No Cookies</span>
                        }
                    </div>

                    <div className="now-playing-body">
                        <img src={status?.nowPlaying?.thumbnail || 'artwork.png'} alt="Track Cover" className="track-thumb" />
                        <div className="track-meta">
                            {status?.nowPlaying ? (
                                <>
                                    <h3>{status.nowPlaying.title}</h3>
                                    <p>{status.nowPlaying.author}</p>
                                    <span className="requested-badge">Requested by {status.nowPlaying.requested}</span>
                                </>
                            ) : (
                                <>
                                    <h3>No Track Playing</h3>
                                    <p style={{ color: 'var(--cf-text-muted)' }}>Idle — Waiting for queue</p>
                                </>
                            )}
                        </div>
                    </div>

                    {/* Audio Spectrum */}
                    <div className="spectrum-bars-container">
                        <div className="spectrum-column"><div className="spectrum-block"></div><div className="spectrum-block"></div></div>
                        <div className="spectrum-column"><div className="spectrum-block"></div><div className="spectrum-block"></div><div className="spectrum-block"></div></div>
                        <div className="spectrum-column"><div className="spectrum-block"></div><div className="spectrum-block"></div></div>
                        <div className="spectrum-column"><div className="spectrum-block"></div><div className="spectrum-block"></div><div className="spectrum-block"></div></div>
                        <div className="spectrum-column"><div className="spectrum-block"></div></div>
                    </div>

                    {/* Player Control Dock */}
                    <div className="player-controls-dock">
                        <div className="control-btn-group" style={{ display: 'flex', gap: '0.6rem', flexWrap: 'wrap' }}>
                            <button onClick={() => handleAction('pause', 'toast_paused', 'info')} className="btn-ctrl">
                                Pause
                            </button>
                            <button onClick={() => handleAction('skip', 'toast_skipped', 'info')} className="btn-ctrl">
                                Skip
                            </button>
                            <button onClick={() => handleAction('stop', 'toast_stopped', 'info')} className="btn-ctrl danger">
                                Stop
                            </button>
                            <button 
                                onClick={() => handleAction('disconnect', 'toast_disconnected', 'error')} 
                                className="btn-ctrl danger"
                                style={{ background: '#991B1B', borderColor: '#7F1D1D' }}
                            >
                                {t('btn_kick')}
                            </button>
                        </div>

                        <div className="volume-control-box">
                            <label>{t('vol_label')}</label>
                            <input 
                                type="range" 
                                min="0" 
                                max="2" 
                                step="0.05" 
                                value={volume} 
                                onChange={(e) => setVolume(parseFloat(e.target.value))} 
                            />
                            <span style={{ fontWeight: 700, color: '#FFF' }}>{Math.round(volume * 100)}%</span>
                        </div>
                    </div>
                </div>

                {/* Right: Cache Gauge Card */}
                <div className="glass-card cache-gauge-card">
                    <div className="stat-header">
                        <span className="stat-title">{t('title_cache')}</span>
                    </div>

                    <div className="gauge-wrapper">
                        <div className="donut-chart-arc">
                            <span className="donut-inner-val">512 MB</span>
                        </div>
                        <span style={{ fontSize: '0.75rem', color: 'var(--cf-text-subtle)', marginTop: '1rem' }}>
                            Limit: 5,120 MB (SQLite WAL Mode)
                        </span>
                    </div>
                </div>
            </div>

            {/* Queue List Section */}
            <div className="glass-card queue-table-card">
                <div className="table-header-row">
                    <h3>{t('title_queue')}</h3>
                    <span className="stat-badge blue">
                        {(status?.nowPlaying ? 1 : 0) + (status?.queue?.length ?? 0)} Active Tracks
                    </span>
                </div>

                <table className="queue-table">
                    <thead>
                        <tr>
                            <th>#</th>
                            <th>{t('th_title')}</th>
                            <th>{t('th_source')}</th>
                            <th>{t('th_duration')}</th>
                            <th>{t('th_requested')}</th>
                            <th>{t('th_status')}</th>
                        </tr>
                    </thead>
                    <tbody>
                        {status?.nowPlaying || (status?.queue && status.queue.length > 0) ? (
                            <>
                                {status?.nowPlaying && (
                                    <tr style={{ background: 'rgba(16, 185, 129, 0.08)' }}>
                                        <td>1</td>
                                        <td>
                                            <div className="track-row-cell">
                                                <strong>{status.nowPlaying.title}</strong>
                                            </div>
                                        </td>
                                        <td>{status.nowPlaying.author}</td>
                                        <td>{status.nowPlaying.duration}</td>
                                        <td>{status.nowPlaying.requested}</td>
                                        <td><span className="status-pill playing">🟢 Playing</span></td>
                                    </tr>
                                )}
                                {status?.queue && status.queue.map((song, i) => (
                                    <tr key={i}>
                                        <td>{(status?.nowPlaying ? 2 : 1) + i}</td>
                                        <td>
                                            <div className="track-row-cell">
                                                <strong>{song.title}</strong>
                                            </div>
                                        </td>
                                        <td>{song.author}</td>
                                        <td>{song.duration}</td>
                                        <td>{song.requested}</td>
                                        <td><span className="status-pill queued">Queued</span></td>
                                    </tr>
                                ))}
                            </>
                        ) : (
                            <tr>
                                <td colSpan={6} style={{ textAlign: 'center', color: 'var(--cf-text-muted)' }}>No tracks in queue</td>
                            </tr>
                        )}
                    </tbody>
                </table>
            </div>
        </section>
    );
};
