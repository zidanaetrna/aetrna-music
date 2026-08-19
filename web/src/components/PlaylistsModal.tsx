import React, { useState, useEffect } from 'react';
import { useToast } from '../context/ToastContext';

interface PlaylistsModalProps {
    token: string;
    selectedGuild: string;
}

interface Collection {
    id: number;
    user_id: string;
    name: string;
    share_code: string;
}

interface CollectionItem {
    id: number;
    collection_id: number;
    title: string;
    url: string;
    source: string;
    duration: number;
    thumbnail: string;
    author: string;
    position: number;
}

export const PlaylistsModal: React.FC<PlaylistsModalProps> = ({ token, selectedGuild }) => {
    const { showToast } = useToast();
    const [playlists, setPlaylists] = useState<Collection[]>([]);
    const [selectedPlaylist, setSelectedPlaylist] = useState<Collection | null>(null);
    const [items, setItems] = useState<CollectionItem[]>([]);
    const [newPlaylistName, setNewPlaylistName] = useState('');
    const [addTrackInput, setAddTrackInput] = useState('');
    const [loading, setLoading] = useState(false);

    const fetchPlaylists = async () => {
        try {
            const res = await fetch('/api/playlists', {
                headers: { 'Authorization': `Bearer ${token}` }
            });
            if (res.ok) {
                const data = await res.json();
                setPlaylists(data || []);
                if (data && data.length > 0 && !selectedPlaylist) {
                    setSelectedPlaylist(data[0]);
                }
            }
        } catch (e) {
            console.error('Failed to fetch playlists:', e);
        }
    };

    const fetchItems = async (playlistId: number) => {
        try {
            const res = await fetch(`/api/playlists/items?collectionId=${playlistId}`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });
            if (res.ok) {
                const data = await res.json();
                setItems(data || []);
            }
        } catch (e) {
            console.error('Failed to fetch playlist items:', e);
        }
    };

    useEffect(() => {
        fetchPlaylists();
    }, [token]);

    useEffect(() => {
        if (selectedPlaylist) {
            fetchItems(selectedPlaylist.id);
        } else {
            setItems([]);
        }
    }, [selectedPlaylist]);

    const handleCreatePlaylist = async () => {
        const name = newPlaylistName.trim();
        if (!name) return showToast('Please enter a playlist name', 'error');

        try {
            const res = await fetch('/api/playlists', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${token}`
                },
                body: JSON.stringify({ name })
            });

            if (res.ok) {
                const newColl = await res.json();
                showToast(`Playlist '${name}' created!`, 'success');
                setNewPlaylistName('');
                fetchPlaylists();
                setSelectedPlaylist(newColl);
            } else {
                showToast('Failed to create playlist', 'error');
            }
        } catch (e) {
            showToast('Error creating playlist', 'error');
        }
    };

    const handleAddTrack = async () => {
        if (!selectedPlaylist) return showToast('Select a playlist first', 'error');
        const query = addTrackInput.trim();
        if (!query) return showToast('Enter a song title or link', 'error');

        setLoading(true);
        try {
            const res = await fetch('/api/playlists/items', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${token}`
                },
                body: JSON.stringify({
                    collectionId: selectedPlaylist.id,
                    title: query,
                    url: query,
                    source: query.includes('spotify') ? 'spotify' : 'youtube',
                    duration: 0,
                    author: 'Custom'
                })
            });

            if (res.ok) {
                showToast(`Added '${query}' to playlist`, 'success');
                setAddTrackInput('');
                fetchItems(selectedPlaylist.id);
            } else {
                showToast('Failed to add track', 'error');
            }
        } catch (e) {
            showToast('Error adding track', 'error');
        } finally {
            setLoading(false);
        }
    };

    const handleDeleteItem = async (itemId: number) => {
        if (!selectedPlaylist) return;
        try {
            const res = await fetch(`/api/playlists/items?collectionId=${selectedPlaylist.id}&itemId=${itemId}`, {
                method: 'DELETE',
                headers: { 'Authorization': `Bearer ${token}` }
            });
            if (res.ok) {
                showToast('Track removed from playlist', 'info');
                fetchItems(selectedPlaylist.id);
            }
        } catch (e) {
            showToast('Error removing track', 'error');
        }
    };

    const handlePlayPlaylistToVoice = async () => {
        if (!selectedPlaylist) return;
        try {
            await fetch('/api/control', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${token}`
                },
                body: JSON.stringify({
                    action: 'load_collection',
                    collectionName: selectedPlaylist.name,
                    guildId: selectedGuild
                })
            });
            showToast(`Loading playlist '${selectedPlaylist.name}' into voice channel...`, 'success');
        } catch (e) {
            showToast('Failed to play playlist', 'error');
        }
    };

    return (
        <section className="tab-page active">
            <div className="header-title-row" style={{ marginBottom: '1.5rem' }}>
                <h2>Custom Saved Playlists</h2>
                <p>Manage your permanent music playlists and enqueue directly into Discord voice channels.</p>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '300px 1fr', gap: '1.5rem' }}>
                {/* Left Panel: Playlist List & Creation */}
                <div className="glass-card" style={{ padding: '1.25rem' }}>
                    <h3 style={{ fontSize: '1.1rem', marginBottom: '1rem' }}>Your Playlists</h3>

                    <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1rem' }}>
                        <input
                            type="text"
                            value={newPlaylistName}
                            onChange={(e) => setNewPlaylistName(e.target.value)}
                            placeholder="New playlist name..."
                            className="cf-search-input"
                            style={{ flex: 1, padding: '0.5rem' }}
                        />
                        <button className="cf-btn-primary" onClick={handleCreatePlaylist} style={{ padding: '0.5rem 0.75rem' }}>
                            + Create
                        </button>
                    </div>

                    <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem', maxHeight: '400px', overflowY: 'auto' }}>
                        {playlists.length === 0 ? (
                            <div style={{ color: '#9CA3AF', fontSize: '0.9rem', textAlign: 'center', padding: '1rem' }}>
                                No saved playlists found.
                            </div>
                        ) : (
                            playlists.map((pl) => (
                                <div
                                    key={pl.id}
                                    onClick={() => setSelectedPlaylist(pl)}
                                    style={{
                                        padding: '0.75rem 1rem',
                                        borderRadius: '8px',
                                        background: selectedPlaylist?.id === pl.id ? 'rgba(88, 101, 242, 0.2)' : 'rgba(255, 255, 255, 0.05)',
                                        border: selectedPlaylist?.id === pl.id ? '1px solid #5865F2' : '1px solid transparent',
                                        cursor: 'pointer',
                                        display: 'flex',
                                        justifyContent: 'space-between',
                                        alignItems: 'center'
                                    }}
                                >
                                    <span style={{ fontWeight: 600, color: selectedPlaylist?.id === pl.id ? '#6366F1' : '#FFF' }}>
                                        {pl.name}
                                    </span>
                                    <span style={{ fontSize: '0.8rem', color: '#9CA3AF' }}>🎵</span>
                                </div>
                            ))
                        )}
                    </div>
                </div>

                {/* Right Panel: Selected Playlist Tracks */}
                <div className="glass-card" style={{ padding: '1.25rem' }}>
                    {selectedPlaylist ? (
                        <>
                            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem', borderBottom: '1px solid rgba(255, 255, 255, 0.1)', paddingBottom: '0.75rem' }}>
                                <div>
                                    <h3 style={{ fontSize: '1.3rem', margin: 0 }}>{selectedPlaylist.name}</h3>
                                    <span style={{ fontSize: '0.85rem', color: '#9CA3AF' }}>{items.length} tracks saved</span>
                                </div>
                                <button className="cf-btn-primary" onClick={handlePlayPlaylistToVoice} style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                                    <span>▶</span> Play in Voice
                                </button>
                            </div>

                            {/* Add Track Input */}
                            <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1.25rem' }}>
                                <input
                                    type="text"
                                    value={addTrackInput}
                                    onChange={(e) => setAddTrackInput(e.target.value)}
                                    placeholder="Enter song name, YouTube URL, or Spotify link..."
                                    className="cf-search-input"
                                    style={{ flex: 1, padding: '0.6rem' }}
                                />
                                <button className="cf-btn-primary" onClick={handleAddTrack} disabled={loading} style={{ padding: '0.6rem 1rem' }}>
                                    {loading ? 'Adding...' : '+ Add Track'}
                                </button>
                            </div>

                            {/* Track List Table */}
                            <div style={{ maxHeight: '350px', overflowY: 'auto' }}>
                                {items.length === 0 ? (
                                    <div style={{ color: '#9CA3AF', textAlign: 'center', padding: '2rem' }}>
                                        This playlist is empty. Add your first song above!
                                    </div>
                                ) : (
                                    <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontSize: '0.9rem' }}>
                                        <thead>
                                            <tr style={{ borderBottom: '1px solid rgba(255, 255, 255, 0.1)', color: '#9CA3AF' }}>
                                                <th style={{ padding: '0.5rem' }}>#</th>
                                                <th style={{ padding: '0.5rem' }}>Title</th>
                                                <th style={{ padding: '0.5rem' }}>Source</th>
                                                <th style={{ padding: '0.5rem', textAlign: 'right' }}>Action</th>
                                            </tr>
                                        </thead>
                                        <tbody>
                                            {items.map((item, idx) => (
                                                <tr key={item.id} style={{ borderBottom: '1px solid rgba(255, 255, 255, 0.05)' }}>
                                                    <td style={{ padding: '0.6rem 0.5rem', color: '#9CA3AF' }}>{idx + 1}</td>
                                                    <td style={{ padding: '0.6rem 0.5rem', fontWeight: 500 }}>
                                                        <a href={item.url} target="_blank" rel="noreferrer" style={{ color: '#ECECF1', textDecoration: 'none' }}>
                                                            {item.title}
                                                        </a>
                                                    </td>
                                                    <td style={{ padding: '0.6rem 0.5rem', textTransform: 'capitalize', color: item.source === 'spotify' ? '#1DB954' : '#FF0000' }}>
                                                        {item.source || 'youtube'}
                                                    </td>
                                                    <td style={{ padding: '0.6rem 0.5rem', textAlign: 'right' }}>
                                                        <button
                                                            onClick={() => handleDeleteItem(item.id)}
                                                            style={{ background: 'none', border: 'none', color: '#EF4444', cursor: 'pointer', fontSize: '0.9rem' }}
                                                            title="Remove track"
                                                        >
                                                            🗑️
                                                        </button>
                                                    </td>
                                                </tr>
                                            ))}
                                        </tbody>
                                    </table>
                                )}
                            </div>
                        </>
                    ) : (
                        <div style={{ color: '#9CA3AF', textAlign: 'center', padding: '3rem' }}>
                            Select or create a playlist to view tracks.
                        </div>
                    )}
                </div>
            </div>
        </section>
    );
};
