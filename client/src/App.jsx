import { useEffect, useState } from 'react'
import { useRoom } from './hooks/useRoom'
import { usePlayback } from './hooks/usePlayback'
import Icon from './components/Icon'
import './App.css'

import Brand from './components/Brand'
import Bars from './components/Bars'
import Welcome from './components/Welcome'
import UploadCard from './components/UploadCard'
import Player from './components/Player'

export default function App() {
  const connection = useRoom()
  const player = usePlayback(connection)
  const {
    room,
    songs,
    session,
    status,
    error,
    setError,
    busy,
    enter,
    upload,
    refreshPlaylist,
    leave,
  } = connection
  const [copied, setCopied] = useState('')
  const connected = status === 'connected'
  const me = room?.members.find((member) => member.id === session?.memberId)
  const online = room?.members.filter((member) => member.connected).length || 0
  useEffect(() => {
    if (!copied) return
    const timer = setTimeout(() => setCopied(''), 2000)
    return () => clearTimeout(timer)
  }, [copied])
  const copy = async (kind) => {
    try {
      const url = new URL(window.location.href)
      url.search = ''
      url.searchParams.set('room', room.code)
      await navigator.clipboard.writeText(
        kind === 'code' ? room.code : url.href,
      )
      setCopied(kind)
    } catch {
      setError(
        'Could not copy automatically. You can select and copy the room code below.',
      )
    }
  }

  return (
    <div className={`app ${room ? 'in-room' : ''}`}>
      <header className="site-header">
        <Brand />
        {room ? (
          <div className="header-actions">
            <span
              className={`connection-badge ${connected ? '' : 'offline'}`}
              role="status"
            >
              <span className="live-dot" />
              {connected ? 'Connected' : 'Reconnecting'}
            </span>
            <button
              className="leave-button"
              aria-label="Leave room"
              onClick={leave}
            >
              <Icon name="leave" size={17} />
              <span>Leave room</span>
            </button>
          </div>
        ) : (
          <span className="header-tagline">A shared listening space.</span>
        )}
      </header>
      {error && (
        <div className="error-banner" role="alert">
          <span>{error}</span>
          <button
            className="icon-button"
            aria-label="Dismiss message"
            onClick={() => setError('')}
          >
            <Icon name="x" size={18} />
          </button>
        </div>
      )}
      {!room && !session && <Welcome enter={enter} busy={busy} />}
      {!room && session && (
        <main className="connecting-screen">
          <span className="spinner" />
          <h1>Finding your rhythm…</h1>
          <p>
            Connecting to your room. A sleeping free server may take a minute to
            wake up.
          </p>
          <button className="button secondary" onClick={leave}>
            Back to the start
          </button>
        </main>
      )}
      {room && (
        <>
          <main className="room-layout">
            <section className="room-intro">
              <div>
                <span className="eyebrow">YOUR SHARED LISTENING SPACE</span>
                <h1>
                  In good company<span>.</span>
                </h1>
                <p>
                  Welcome, {me?.name || 'music lover'}. Settle in and find your
                  next favorite.
                </p>
              </div>
              <div className="room-number">
                <span>ROOM CODE</span>
                <button
                  onClick={() => copy('code')}
                  aria-label="Copy room code"
                >
                  <strong>{room.code}</strong>
                  <Icon name={copied === 'code' ? 'check' : 'copy'} size={18} />
                </button>
              </div>
            </section>
            <aside className="left-column">
              <UploadCard
                upload={upload}
                disabled={!connected}
                setError={setError}
              />
              <section className="invite-panel">
                <div className="invite-icon">
                  <Icon name="link" size={19} />
                </div>
                <h2>Better with friends.</h2>
                <p>
                  Send them your room link.
                  <br />
                  We’ll take care of the sync.
                </p>
                <button className="text-button" onClick={() => copy('link')}>
                  {copied === 'link' ? 'Link copied' : 'Copy invite link'}
                  <Icon
                    name={copied === 'link' ? 'check' : 'arrow'}
                    size={17}
                  />
                </button>
              </section>
              <p className="retention-note">
                Room tracks are temporary and are removed after the room
                expires.
              </p>
            </aside>
            <section className="playlist panel">
              <div className="playlist-heading">
                <div>
                  <span className="eyebrow">THE ROOM’S ROTATION</span>
                  <h2>On the playlist</h2>
                </div>
                <div className="playlist-actions">
                  <button
                    className="icon-button"
                    aria-label="Refresh playlist"
                    onClick={refreshPlaylist}
                    disabled={!connected}
                  >
                    <Icon name="refresh" size={16} />
                  </button>
                  <span className="count-badge">
                    {songs.length} {songs.length === 1 ? 'track' : 'tracks'}
                  </span>
                </div>
              </div>
              <div className="track-column-labels">
                <span>#</span>
                <span>TRACK</span>
                <span>SOURCE</span>
              </div>
              {songs.length === 0 ? (
                <div className="empty-playlist">
                  <span>
                    <Icon name="music" size={34} />
                  </span>
                  <h3>Every good room starts with a song.</h3>
                  <p>Upload a favorite to get everyone in sync.</p>
                </div>
              ) : (
                <div className="track-list">
                  {songs.map((song, index) => {
                    const active = song.id === room.playback.songId
                    return (
                      <button
                        className={`track ${active ? 'active' : ''}`}
                        key={song.id}
                        onClick={() => player.select(song.id)}
                        disabled={!connected}
                        aria-pressed={active}
                        aria-label={`Play ${song.name}`}
                      >
                        <span className="track-number">
                          {active && player.playing ? (
                            <Bars small />
                          ) : (
                            <>
                              <span className="track-index">
                                {String(index + 1).padStart(2, '0')}
                              </span>
                              <Icon
                                name="play"
                                size={15}
                                className="track-play"
                              />
                            </>
                          )}
                        </span>
                        <span className="track-description">
                          <strong>
                            {song.name.replace(/\.(mp3|wav|ogg)$/i, '')}
                          </strong>
                          <span>
                            {song.contentType === 'audio/mpeg'
                              ? 'MP3'
                              : song.contentType
                                  .split('/')
                                  .pop()
                                  .toUpperCase()}{' '}
                            · {(song.size / 1024 / 1024).toFixed(1)} MB
                          </span>
                        </span>
                        <span
                          className={`source-tag ${song.default ? 'default' : ''}`}
                        >
                          {song.default ? 'House pick' : 'Room upload'}
                        </span>
                      </button>
                    )
                  })}
                </div>
              )}
              <div className="playlist-footnote">
                <Icon name="headphones" size={16} />
                <span>Seek together. Pause just for you.</span>
              </div>
            </section>
            <aside className="right-column">
              <section className="panel members-panel">
                <div className="members-heading">
                  <h2>The listening party</h2>
                  <span className="count-badge">{online}</span>
                </div>
                <p className="muted">
                  <span className="live-dot" />{' '}
                  {online === 1
                    ? 'A little company is one invite away.'
                    : 'Different places. Same soundtrack.'}
                </p>
                <ul className="member-list">
                  {room.members.map((member, index) => (
                    <li key={member.id}>
                      <span className={`avatar avatar-${index % 4}`}>
                        {member.name
                          .split(' ')
                          .map((part) => part[0])
                          .join('')}
                        <span
                          className={`member-dot ${member.connected ? '' : 'disconnected'}`}
                        />
                      </span>
                      <div>
                        <strong>{member.name}</strong>
                        <span>
                          {member.connected ? 'In the room' : 'Reconnecting'}
                        </span>
                      </div>
                      {member.id === session?.memberId && (
                        <span className="you-badge">you</span>
                      )}
                    </li>
                  ))}
                </ul>
              </section>
              <section className="sync-note">
                <span className="sync-symbol">
                  <Bars small active={connected} />
                </span>
                <h3>On the same wavelength.</h3>
                <p>
                  Seeks update everyone. Need a break? Pause locally, then press
                  play to catch up with the room.
                </p>
              </section>
            </aside>
          </main>
          <audio ref={player.audio} preload="auto" />
          <Player player={player} connected={connected} />
        </>
      )}
      {!room && (
        <footer className="welcome-footer">
          <span>Made for the moments between the songs.</span>
          <span>Listen. Together.</span>
        </footer>
      )}
    </div>
  )
}
