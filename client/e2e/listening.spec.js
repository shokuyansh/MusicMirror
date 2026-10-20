import { test, expect } from '@playwright/test'

function wave(seconds = 120) {
  const sampleRate = 8000
  const buffer = Buffer.alloc(44 + seconds * sampleRate * 2)
  buffer.write('RIFF', 0)
  buffer.writeUInt32LE(buffer.length - 8, 4)
  buffer.write('WAVEfmt ', 8)
  buffer.writeUInt32LE(16, 16)
  buffer.writeUInt16LE(1, 20)
  buffer.writeUInt16LE(1, 22)
  buffer.writeUInt32LE(sampleRate, 24)
  buffer.writeUInt32LE(sampleRate * 2, 28)
  buffer.writeUInt16LE(2, 32)
  buffer.writeUInt16LE(16, 34)
  buffer.write('data', 36)
  buffer.writeUInt32LE(buffer.length - 44, 40)
  for (let i = 0; i < seconds * sampleRate; i++)
    buffer.writeInt16LE(
      Math.round(Math.sin((i * 440 * 2 * Math.PI) / sampleRate) * 300),
      44 + i * 2,
    )
  return buffer
}

async function create(page) {
  await page.goto('/')
  await page.getByRole('button', { name: 'Create a room' }).click()
  await expect(page.getByText('Connected', { exact: true })).toBeVisible()
  return (
    await page.getByRole('button', { name: 'Copy room code' }).innerText()
  ).trim()
}

async function upload(page, name = 'Shared melody.wav') {
  await page
    .getByLabel('Choose audio file')
    .setInputFiles({ name, mimeType: 'audio/wav', buffer: wave() })
  await page.getByRole('button', { name: 'Add to playlist' }).click()
  await expect(page.getByRole('button', { name: `Play ${name}` })).toBeVisible()
}

async function join(page, code) {
  await page.goto(`/?room=${code}`)
  await expect(page.getByLabel('ROOM CODE')).toHaveValue(code)
  await page.getByRole('button', { name: 'Join a room' }).click()
  await expect(page.getByText('Connected', { exact: true })).toBeVisible()
}

async function playing(page) {
  await expect
    .poll(() =>
      page
        .locator('audio')
        .evaluate((audio) => !audio.paused && audio.currentTime > 0),
    )
    .toBe(true)
}

async function seek(page, seconds) {
  const range = page.getByRole('slider', { name: 'Seek for everyone' })
  await range.fill(String(seconds))
  await range.dispatchEvent('pointerup')
}

test('two listeners seek together, pause locally, resume in sync, and rejoin after reload', async ({
  page,
  context,
}) => {
  const errors = []
  page.on('pageerror', (error) => errors.push(error.message))
  const code = await create(page)
  await upload(page)
  const guest = await context.newPage()
  guest.on('pageerror', (error) => errors.push(error.message))
  await join(guest, code)
  await expect(
    guest.getByRole('button', { name: 'Play Shared melody.wav' }),
  ).toBeVisible()
  await page.getByRole('button', { name: 'Play Shared melody.wav' }).click()
  await playing(page)
  await playing(guest)
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.screenshot({
    path: 'test-results/desktop-room.png',
    fullPage: true,
  })
  await seek(page, 30)
  await expect
    .poll(() => guest.locator('audio').evaluate((audio) => audio.currentTime))
    .toBeGreaterThan(29.8)
  await guest.getByRole('button', { name: 'Pause for me' }).click()
  await expect
    .poll(() => guest.locator('audio').evaluate((audio) => audio.paused))
    .toBe(true)
  await expect
    .poll(() => page.locator('audio').evaluate((audio) => audio.paused))
    .toBe(false)
  await seek(page, 60)
  await expect
    .poll(() => guest.locator('audio').evaluate((audio) => audio.currentTime))
    .toBeGreaterThan(59.8)
  await expect
    .poll(() => guest.locator('audio').evaluate((audio) => audio.paused))
    .toBe(true)
  await guest.getByRole('button', { name: 'Resume in sync' }).click()
  await playing(guest)
  // Measure both clocks in parallel rather than adding sequential browser latency.
  await expect
    .poll(async () => {
      const [a, b] = await Promise.all([
        page.locator('audio').evaluate((audio) => audio.currentTime),
        guest.locator('audio').evaluate((audio) => audio.currentTime),
      ])
      return Math.abs(a - b)
    })
    .toBeLessThan(0.35)
  await guest.reload()
  await expect(guest.getByText('Connected', { exact: true })).toBeVisible()
  await playing(guest)
  await expect(page.locator('.member-list li')).toHaveCount(2)
  await guest.getByRole('button', { name: 'Leave room' }).click()
  await expect(
    guest.getByRole('button', { name: 'Create a room' }),
  ).toBeVisible()
  await expect(page.locator('.member-list li')).toHaveCount(1)
  expect(errors).toEqual([])
})

test('late join aligns playback and a second upload updates everyone', async ({
  page,
  context,
}) => {
  const code = await create(page)
  await upload(page)
  await page.getByRole('button', { name: 'Play Shared melody.wav' }).click()
  await playing(page)
  await seek(page, 45)
  const guest = await context.newPage()
  // Delay both directions by 120 ms. Clock estimation should absorb the RTT.
  await guest.routeWebSocket('**/api/ws', (socket) => {
    const server = socket.connectToServer()
    let closed = false
    socket.onMessage((message) =>
      setTimeout(() => {
        if (!closed) server.send(message)
      }, 120),
    )
    server.onMessage((message) =>
      setTimeout(() => {
        if (!closed) socket.send(message)
      }, 120),
    )
    socket.onClose((code, reason) => {
      closed = true
      server.close({ code, reason })
    })
    server.onClose((code, reason) => {
      closed = true
      socket.close({ code, reason })
    })
  })
  await join(guest, code)
  await playing(guest)
  await expect
    .poll(() => guest.locator('audio').evaluate((audio) => audio.currentTime))
    .toBeGreaterThan(44.8)
  await expect
    .poll(async () => {
      const [a, b] = await Promise.all([
        page.locator('audio').evaluate((audio) => audio.currentTime),
        guest.locator('audio').evaluate((audio) => audio.currentTime),
      ])
      return Math.abs(a - b)
    })
    .toBeLessThan(0.35)
  await upload(page, 'Second melody.wav')
  await expect(
    guest.getByRole('button', { name: 'Play Second melody.wav' }),
  ).toBeVisible()
  await guest.getByRole('button', { name: 'Pause for me' }).click()
  await page.getByRole('button', { name: 'Play Second melody.wav' }).click()
  await expect(guest.locator('.track.active')).toContainText('Second melody')
  await expect
    .poll(() => guest.locator('audio').evaluate((audio) => audio.paused))
    .toBe(true)
})

test('autoplay denial offers an explicit resume action', async ({ page }) => {
  await page.addInitScript(() => {
    const original = HTMLMediaElement.prototype.play
    window.allowAudio = false
    HTMLMediaElement.prototype.play = function () {
      return window.allowAudio
        ? original.call(this)
        : Promise.reject(
            new DOMException('Gesture required', 'NotAllowedError'),
          )
    }
  })
  await create(page)
  await upload(page)
  await page.getByRole('button', { name: 'Play Shared melody.wav' }).click()
  await expect(page.getByText('Tap play to enable audio')).toBeVisible()
  await page.evaluate(() => {
    window.allowAudio = true
  })
  await page.getByRole('button', { name: 'Resume in sync' }).click()
  await playing(page)
})

test('mobile flow fits the viewport and invalid room codes show an error', async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')
  await page.getByLabel('ROOM CODE').fill('000000')
  await page.getByRole('button', { name: 'Join a room' }).click()
  await expect(page.getByRole('alert')).toContainText('room does not exist')
  await page.getByRole('button', { name: 'Create a room' }).click()
  await expect(page.getByText('Connected', { exact: true })).toBeVisible()
  const fits = await page.evaluate(
    () => document.documentElement.scrollWidth <= window.innerWidth,
  )
  expect(fits).toBe(true)
  await page.screenshot({
    path: 'test-results/mobile-room.png',
    fullPage: true,
  })
})

test('socket reconnect preserves a local pause and resyncs on resume', async ({
  page,
  context,
}) => {
  await page.addInitScript(() => {
    const Native = window.WebSocket
    window.testSockets = []
    window.WebSocket = class extends Native {
      constructor(...args) {
        super(...args)
        window.testSockets.push(this)
      }
    }
  })
  const code = await create(page)
  await upload(page)
  const guest = await context.newPage()
  await join(guest, code)
  await page.getByRole('button', { name: 'Play Shared melody.wav' }).click()
  await playing(page)
  await playing(guest)
  await page.getByRole('button', { name: 'Pause for me' }).click()
  await page.evaluate(() => window.testSockets.at(-1).close())
  await expect(page.getByText('Reconnecting', { exact: true })).toBeVisible()
  await expect(page.getByText('Connected', { exact: true })).toBeVisible()
  await expect(page.locator('.member-list li')).toHaveCount(2)
  await expect
    .poll(() => page.locator('audio').evaluate((audio) => audio.paused))
    .toBe(true)
  await page.getByRole('button', { name: 'Resume in sync' }).click()
  await playing(page)
  await expect
    .poll(async () => {
      const [a, b] = await Promise.all([
        page.locator('audio').evaluate((audio) => audio.currentTime),
        guest.locator('audio').evaluate((audio) => audio.currentTime),
      ])
      return Math.abs(a - b)
    })
    .toBeLessThan(0.35)
})

test('simultaneous end-of-track events advance the room once', async ({
  page,
  context,
}) => {
  const code = await create(page)
  await upload(page)
  await upload(page, 'Next melody.wav')
  const guest = await context.newPage()
  await join(guest, code)
  await page.getByRole('button', { name: 'Play Shared melody.wav' }).click()
  await playing(page)
  await playing(guest)
  await Promise.all([
    page.locator('audio').dispatchEvent('ended'),
    guest.locator('audio').dispatchEvent('ended'),
  ])
  await expect(page.locator('.track.active')).toContainText('Next melody')
  await expect(guest.locator('.track.active')).toContainText('Next melody')
  await playing(page)
  await playing(guest)
})

test('drift recovery waits for buffered audio instead of starting another download', async ({
  page,
}) => {
  await create(page)
  await upload(page)
  await page.getByRole('button', { name: 'Play Shared melody.wav' }).click()
  await playing(page)
  await seek(page, 30)
  await expect
    .poll(() => page.locator('audio').evaluate((audio) => audio.currentTime))
    .toBeGreaterThan(29.8)
  await expect
    .poll(() =>
      page
        .locator('audio')
        .evaluate((audio) => !audio.seeking && audio.readyState >= 3),
    )
    .toBe(true)

  // Emulate a listener emerging four seconds behind after a slow range fetch.
  // Native playback continues, but the reported buffer has only 100 ms ahead.
  // An automatic jump beyond that buffer would initiate another range request.
  await page.locator('audio').evaluate((audio) => {
    const descriptor = Object.getOwnPropertyDescriptor(
      HTMLMediaElement.prototype,
      'currentTime',
    )
    let lag = 4
    window.recoverySeeks = []
    window.recoveryBufferReady = false
    Object.defineProperty(audio, 'currentTime', {
      configurable: true,
      get() {
        return descriptor.get.call(audio) - lag
      },
      set(value) {
        window.recoverySeeks.push(value)
        lag = 0
        descriptor.set.call(audio, value)
      },
    })
    Object.defineProperty(audio, 'buffered', {
      configurable: true,
      get() {
        return {
          length: 1,
          start: () => 0,
          end: () =>
            window.recoveryBufferReady ? 120 : audio.currentTime + 0.1,
        }
      },
    })
  })
  // Wait for several correction intervals while there is insufficient buffer.
  await expect
    .poll(() => page.locator('audio').evaluate((audio) => audio.currentTime))
    .toBeGreaterThan(26.8)
  expect(await page.evaluate(() => window.recoverySeeks.length)).toBe(0)
  expect(await page.locator('audio').evaluate((audio) => audio.paused)).toBe(
    false,
  )
  await page.evaluate(() => {
    window.recoveryBufferReady = true
  })
  await expect
    .poll(() => page.evaluate(() => window.recoverySeeks.length))
    .toBe(1)
  await expect
    .poll(() => page.locator('audio').evaluate((audio) => audio.currentTime))
    .toBeGreaterThan(30)
  // Explicit room seeks are still allowed to request an unbuffered position.
  await page.evaluate(() => {
    window.recoveryBufferReady = false
  })
  await seek(page, 80)
  await expect
    .poll(() => page.locator('audio').evaluate((audio) => audio.currentTime))
    .toBeGreaterThan(79.8)
  expect(await page.evaluate(() => window.recoverySeeks.length)).toBe(2)
})
