import React, { useState } from 'react';
import { useI18n } from '../context/I18nContext';
import { useToast } from '../context/ToastContext';

interface LoginModalProps {
    onLoginSuccess: (token: string) => void;
}

export const LoginModal: React.FC<LoginModalProps> = ({ onLoginSuccess }) => {
    const { t } = useI18n();
    const { showToast } = useToast();
    const [password, setPassword] = useState('');
    const [showPassword, setShowPassword] = useState(false);
    const [errorMsg, setErrorMsg] = useState('');
    const [copyText, setCopyText] = useState('Copy');

    const handleLogin = async (e: React.FormEvent) => {
        e.preventDefault();
        setErrorMsg('');

        try {
            const res = await fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password })
            });

            if (!res.ok) {
                setErrorMsg('Invalid password. Please check your .env configuration.');
                return;
            }

            const data = await res.json();
            onLoginSuccess(data.token);
            showToast(t('toast_auth_success'), 'success');
        } catch (err) {
            setErrorMsg('Network error — unable to reach the server. Please try again.');
        }
    };

    const handleCopyWallet = () => {
        const wallet = "0xA1a4d3F3A49f4514CCEE434Cfc66837A1fFC687d";
        navigator.clipboard.writeText(wallet).then(() => {
            setCopyText('Copied!');
            showToast(t('toast_wallet_copied'), 'success');
            setTimeout(() => setCopyText('Copy'), 2000);
        });
    };

    return (
        <div className="auth-split-screen">
            {/* Left Side: Clean Form Column */}
            <div className="auth-form-column">
                <div className="auth-form-inner">
                    <div className="auth-brand">
                        <img src="artwork.png" alt="Aetrna Logo" className="auth-brand-img" />
                        <div className="auth-brand-meta">
                            <span className="auth-brand-name">Aetrna's Music</span>
                            <span className="artist-credit">Artwork by <strong>@br_lie</strong></span>
                        </div>
                    </div>

                    <div className="auth-header">
                        <h2>{t('welcome_back')}</h2>
                        <p>{t('sign_in_desc')}</p>
                    </div>

                    <form onSubmit={handleLogin} className="split-auth-form">
                        <div className="form-group">
                            <label>{t('pass_label')}</label>
                            <div className="input-with-icon">
                                <span className="input-icon">
                                    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2">
                                        <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
                                        <path d="M7 11V7a5 5 0 0110 0v4"/>
                                    </svg>
                                </span>
                                <input 
                                    type={showPassword ? 'text' : 'password'} 
                                    value={password}
                                    onChange={(e) => setPassword(e.target.value)}
                                    placeholder="Enter your password" 
                                    required 
                                    autoComplete="current-password"
                                />
                                <button 
                                    type="button" 
                                    onClick={() => setShowPassword(!showPassword)}
                                    className="toggle-eye"
                                >
                                    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2">
                                        <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                                        <circle cx="12" cy="12" r="3"/>
                                    </svg>
                                </button>
                            </div>
                        </div>

                        <button type="submit" className="btn btn-split-submit">{t('btn_signin')}</button>
                    
                        <div style={{ textAlign: 'center', marginTop: '0.75rem' }}>
                            <span style={{ fontSize: '0.72rem', color: '#94A3B8', fontWeight: 500 }}>
                                Aetrna's Music v2.1.8
                            </span>
                        </div>
                    </form>

                    {errorMsg && (
                        <div className="auth-error-alert" style={{ marginTop: '1rem', color: '#F43F5E', fontSize: '0.85rem' }}>
                            <span>{errorMsg}</span>
                        </div>
                    )}
                </div>
            </div>

            {/* Right Side: Deep Emerald Green Showcase Column */}
            <div className="auth-hero-column">
                <div className="auth-hero-content">
                    <h1>Revolutionize Discord Music with Native Hybrid Engine</h1>
                    
                    <div className="testimonial-card">
                        <span className="quote-mark">“</span>
                        <p className="testimonial-text">“Gua cuma Professional AI Prompter. Kalo kodenya agak ajaib tapi lagunya muter lancar jaya, berarti prompt gua gacor.”</p>
                        
                        <div className="author-profile">
                            <img src="https://github.com/zidanaetrna.png" alt="zidanaetrna" className="author-avatar" />
                            <div className="author-info">
                                <span className="author-name">zidanaetrna</span>
                                <span className="author-role">Professional AI Prompter</span>
                            </div>
                        </div>
                    </div>

                    {/* Tech Ecosystem Section with Official SVG Vector Logos */}
                    <div className="tech-ecosystem-section">
                        <span className="tech-section-title">POWERED BY MODERN TECH STACK</span>
                        <div className="tech-logo-row">
                            {/* Discord Logo */}
                            <span className="tech-logo-item" title="Discord API Gateway">
                                <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
                                    <path d="M20.317 4.37a19.791 19.791 0 00-4.885-1.515.074.074 0 00-.079.037c-.21.375-.444.864-.608 1.25a18.27 18.27 0 00-5.487 0 12.64 12.64 0 00-.617-1.25.077.077 0 00-.079-.037A19.736 19.736 0 003.677 4.37a.07.07 0 00-.032.027C.533 9.046-.32 13.58.099 18.057a.082.082 0 00.031.057 19.9 19.9 0 005.993 3.03.078.078 0 00.084-.028c.462-.63.874-1.295 1.226-1.994.021-.041.001-.09-.041-.106a13.107 13.107 0 01-1.872-.892.077.077 0 01-.008-.128 10.2 10.2 0 00.372-.292.074.074 0 01.077-.01c3.928 1.793 8.18 1.793 12.061 0a.074.074 0 01.078.01c.12.098.246.198.373.292a.077.077 0 01-.006.127 12.299 12.299 0 01-1.873.893.077.077 0 00-.041.107c.36.698.772 1.362 1.225 1.993a.076.076 0 00.084.028 19.839 19.839 0 006.002-3.03.077.077 0 00.032-.054c.5-5.177-.838-9.674-3.549-13.66a.061.061 0 00-.031-.028zM8.02 15.33c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.956-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.956 2.418-2.157 2.418zm7.975 0c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.955-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.946 2.418-2.157 2.418z"/>
                                </svg>
                                Discord API
                            </span>

                            {/* Go Logo */}
                            <span className="tech-logo-item" title="Go 1.23 Microservice Core">
                                <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
                                    <path d="M1.811 10.231c-.046 0-.093 0-.139.012-.662.139-.906.778-.545 1.36.313.51.987.674 1.58.384.603-.29.835-.94.51-1.474-.244-.407-.812-.293-1.406-.282zm20.378.012c-.594-.011-1.162-.125-1.406.282-.325.534-.093 1.184.51 1.474.593.29 1.267.126 1.58-.384.361-.582.117-1.221-.545-1.36a.669.669 0 00-.139-.012zM12 4.5c-4.142 0-7.5 3.358-7.5 7.5s3.358 7.5 7.5 7.5 7.5-3.358 7.5-7.5-3.358-7.5-7.5-7.5zm0 12c-2.485 0-4.5-2.015-4.5-4.5S9.515 7.5 12 7.5s4.5 2.015 4.5 4.5-2.015 4.5-4.5 4.5z"/>
                                </svg>
                                Go 1.23
                            </span>

                            {/* Node.js Logo */}
                            <span className="tech-logo-item" title="Node.js 22 Voice Gateway">
                                <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
                                    <path d="M12 2L2 7.5v9L12 22l10-5.5v-9L12 2zm8 13.5L12 20l-8-4.5V8.5L12 4l8 4.5v7zM12 7L6 10.3v3.4l6 3.3 6-3.3v-3.4L12 7z"/>
                                </svg>
                                Node.js 22
                            </span>

                            {/* yt-dlp Logo */}
                            <span className="tech-logo-item" title="yt-dlp Stream Extractor Engine">
                                <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2">
                                    <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4M7 10l5 5 5-5M12 15V3"/>
                                </svg>
                                yt-dlp
                            </span>

                            {/* FFmpeg Audio Processing Logo */}
                            <span className="tech-logo-item" title="FFmpeg Audio Processing & DSP Engine">
                                <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2">
                                    <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>
                                    <path d="M19.07 4.93a10 10 0 010 14.14M15.54 8.46a5 5 0 010 7.07"/>
                                </svg>
                                FFmpeg
                            </span>

                            {/* SQLite3 Logo */}
                            <span className="tech-logo-item" title="SQLite3 WAL Mode Cache Database">
                                <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2">
                                    <ellipse cx="12" cy="5" rx="9" ry="3"/>
                                    <path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/>
                                    <path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/>
                                </svg>
                                SQLite3
                            </span>
                        </div>
                    </div>

                    <div className="donation-box">
                        <span className="donation-title">SUPPORT THE PROJECT (EVM DONATION)</span>
                        <div className="donation-input-row">
                            <input type="text" readOnly value="0xA1a4d3F3A49f4514CCEE434Cfc66837A1fFC687d" className="donation-wallet-text" />
                            <button type="button" onClick={handleCopyWallet} className="btn-copy-wallet">
                                {copyText}
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
};
