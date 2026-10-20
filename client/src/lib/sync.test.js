import test from 'node:test'
import assert from 'node:assert/strict'
import { RoomClock, correction, positionAt, formatTime } from './sync.js'

test('clock estimates server time using the lowest round-trip sample', () => {
  let now = 1000
  const clock = new RoomClock(() => now)
  clock.observe(100000, 900, 1100)
  assert.equal(clock.now(), 100000)
  clock.observe(100500, 1400, 1420)
  now = 1500
  assert.equal(clock.now(), 100590)
  clock.observe(999999, 1400, 1800)
  assert.equal(clock.now(), 100590)
  assert.equal(clock.observe(1000, 5000, 4000), false)
  assert.equal(clock.observe(NaN, 1000, 1100), false)
})

test('seeks and resumes derive position from authoritative state', () => {
  const playback = { songId: 'song', position: 42, updatedAt: 100000 }
  assert.equal(positionAt(playback, 105000), 47)
  assert.equal(positionAt(playback, 110000, 48), 48)
  assert.equal(positionAt({ ...playback, position: 0 }, 99000), 0)
  assert.equal(positionAt(null, 100000), 0)
  // A paused client's old audio time does not influence resuming at room time.
  assert.equal(positionAt(playback, 160000), 102)
})

test('drift correction is bounded and avoids repeated small jumps', () => {
  assert.deepEqual(correction(10, 10.02), { seek: null, rate: 1 })
  assert.equal(correction(10, 10.2).seek, null)
  assert.ok(correction(10, 10.2).rate > 1)
  assert.ok(correction(10, 9.8).rate < 1)
  assert.deepEqual(correction(10, 12), { seek: 12, rate: 1 })
  assert.deepEqual(correction(10, 8), { seek: 8, rate: 1 })
  assert.equal(formatTime(NaN), '0:00')
  assert.equal(formatTime(125), '2:05')
})
