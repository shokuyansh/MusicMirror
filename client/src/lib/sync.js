// Use the monotonic browser clock, independent of changes to the device's date.
export class RoomClock {
  constructor(now = () => performance.now()) {
    this.monotonic = now
    this.offset = null
    this.samples = []
  }

  seed(serverTime, sent, received = this.monotonic()) {
    if (this.offset === null) this.observe(serverTime, sent, received)
  }

  observe(serverTime, sent, received = this.monotonic()) {
    const rtt = received - sent
    if (
      ![serverTime, sent, received].every(Number.isFinite) ||
      rtt < 0 ||
      rtt > 10000
    )
      return false
    this.samples.push({ rtt, offset: serverTime - (sent + received) / 2 })
    this.samples = this.samples.slice(-12)
    this.offset = this.samples.reduce((best, sample) =>
      sample.rtt < best.rtt ? sample : best,
    ).offset
    return true
  }

  now() {
    return this.monotonic() + (this.offset ?? Date.now() - this.monotonic())
  }
}

export function positionAt(playback, serverTime, duration = Infinity) {
  if (!playback?.songId) return 0
  const target = playback.position + (serverTime - playback.updatedAt) / 1000
  return Math.max(
    0,
    Math.min(Number.isFinite(duration) ? duration : Infinity, target),
  )
}

// A drift correction must not restart buffering by jumping to bytes that have
// not arrived yet. Keep a little audio ahead of the target so playback can resume.
export function bufferedAt(ranges, target, duration = Infinity) {
  const end = Math.min(target + 0.75, duration)
  for (let i = 0; i < ranges.length; i++) {
    if (ranges.start(i) <= target && ranges.end(i) >= end) return true
  }
  return false
}

export function correction(current, target, canSeek = true) {
  const drift = target - current
  if (Math.abs(drift) > 0.3 && canSeek) return { seek: target, rate: 1 }
  if (Math.abs(drift) < 0.04) return { seek: null, rate: 1 }
  return { seek: null, rate: Math.max(0.97, Math.min(1.03, 1 + drift * 0.15)) }
}

export function formatTime(seconds) {
  if (!Number.isFinite(seconds) || seconds < 0) return '0:00'
  return `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, '0')}`
}
