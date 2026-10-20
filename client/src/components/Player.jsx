import { useEffect, useState } from 'react'
import Icon from './Icon'
import { formatTime } from '../lib/sync'

export default function Player({ player, connected }) {
  const [draft, setDraft] = useState(null)
  const commit = () => {
    if (draft !== null) {
      player.seek(draft)
      setDraft(null)
    }
  }
  useEffect(() => setDraft(null), [player.currentSong?.id])
  const title =
    player.currentSong?.name.replace(/\.(mp3|wav|ogg)$/i, '') ||
    'Your soundtrack starts here'
  return (
    <footer className="player" aria-label="Music player">
      <div className={`player-art ${player.playing ? 'is-playing' : ''}`}>
        <Icon name="music" size={24} />
      </div>
      <div className="player-info">
        <strong title={title}>{title}</strong>
        <span>
          {player.blocked
            ? 'Tap play to enable audio'
            : player.paused
              ? 'Paused for you · room keeps playing'
              : player.loading
                ? 'Buffering your track…'
                : player.currentSong
                  ? 'Listening together'
                  : 'Choose a track from the playlist'}
        </span>
      </div>
      <div className="player-center">
        <div className="transport">
          <button
            className="icon-button"
            aria-label="Previous song"
            onClick={() => player.skip(-1)}
            disabled={!connected || !player.currentSong}
          >
            <Icon name="previous" />
          </button>
          <button
            className="play-button"
            aria-label={player.playing ? 'Pause for me' : 'Resume in sync'}
            onClick={player.toggle}
            disabled={!player.currentSong || (!connected && !player.playing)}
          >
            {player.loading && !player.playing ? (
              <span className="spinner" />
            ) : (
              <Icon name={player.playing ? 'pause' : 'play'} size={23} />
            )}
          </button>
          <button
            className="icon-button"
            aria-label="Next song"
            onClick={() => player.skip(1)}
            disabled={!connected || !player.currentSong}
          >
            <Icon name="next" />
          </button>
        </div>
        <div className="progress">
          <span>{formatTime(draft ?? player.currentTime)}</span>
          <input
            aria-label="Seek for everyone"
            type="range"
            min="0"
            max={player.duration || 0}
            step="0.1"
            value={draft ?? Math.min(player.currentTime, player.duration)}
            onChange={(event) => setDraft(Number(event.target.value))}
            onPointerUp={commit}
            onPointerCancel={() => setDraft(null)}
            onKeyUp={(event) => {
              if (
                [
                  'ArrowLeft',
                  'ArrowRight',
                  'Home',
                  'End',
                  'PageUp',
                  'PageDown',
                ].includes(event.key)
              )
                commit()
            }}
            onBlur={commit}
            disabled={!connected || !player.duration}
            style={{
              '--progress': `${player.duration ? ((draft ?? player.currentTime) / player.duration) * 100 : 0}%`,
            }}
          />
          <span>{formatTime(player.duration)}</span>
        </div>
      </div>
      <div className="volume">
        <button
          className="icon-button"
          aria-label={player.volume ? 'Mute for me' : 'Unmute for me'}
          onClick={() => player.setVolume(player.volume ? 0 : 0.8)}
        >
          <Icon name={player.volume ? 'volume' : 'mute'} size={18} />
        </button>
        <input
          aria-label="Volume for me"
          type="range"
          min="0"
          max="1"
          step="0.01"
          value={player.volume}
          onChange={(event) => player.setVolume(Number(event.target.value))}
        />
      </div>
    </footer>
  )
}
