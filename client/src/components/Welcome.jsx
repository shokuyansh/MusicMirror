import { useState } from 'react'
import Icon from './Icon'
import Bars from './Bars'

export default function Welcome({ enter, busy }) {
  const [code, setCode] = useState(
    () =>
      new URLSearchParams(window.location.search).get('room')?.slice(0, 6) ||
      '',
  )
  return (
    <main className="welcome">
      <section className="welcome-copy">
        <span className="eyebrow">
          <span className="live-dot" /> GOOD MUSIC. BETTER COMPANY.
        </span>
        <h1>
          Your people.
          <br />
          Your <span>soundtrack.</span>
        </h1>
        <p>
          Different places, same moment. Create a room, bring your favorite
          tracks, and listen together.
        </p>
        <div className="welcome-record">
          <div className="record">
            <div className="record-label">
              <Icon name="music" size={28} />
            </div>
          </div>
          <div className="record-caption">
            <span>ON THE SAME WAVELENGTH</span>
            <Bars active={!busy} />
          </div>
        </div>
      </section>
      <section
        className="entry-card"
        aria-label="Create or join a listening room"
      >
        <span className="card-icon">
          <Icon name="headphones" size={26} />
        </span>
        <h2>A room for your rhythm.</h2>
        <p>No accounts. Just a room code and good taste.</p>
        <button
          className="button primary"
          onClick={() => enter()}
          disabled={busy}
        >
          {busy ? <span className="spinner" /> : <Icon name="music" />} Create a
          room <Icon name="arrow" />
        </button>
        <div className="divider">
          <span>or join your friends</span>
        </div>
        <form
          onSubmit={(event) => {
            event.preventDefault()
            enter(code.trim().toLowerCase())
          }}
        >
          <label htmlFor="room-code">ROOM CODE</label>
          <input
            id="room-code"
            className="code-input"
            value={code}
            onChange={(event) =>
              setCode(
                event.target.value.replace(/[^a-fA-F0-9]/g, '').slice(0, 6),
              )
            }
            placeholder="a1b2c3"
            pattern="[a-fA-F0-9]{6}"
            maxLength={6}
            minLength={6}
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            required
            disabled={busy}
          />
          <button
            className="button secondary"
            type="submit"
            disabled={busy || code.length !== 6}
          >
            Join a room <Icon name="arrow" />
          </button>
        </form>
        <p className="entry-footnote">
          <Icon name="users" size={15} /> A shared space for up to 20 listeners
        </p>
      </section>
    </main>
  )
}
