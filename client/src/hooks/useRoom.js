import { useCallback, useEffect, useRef, useState } from 'react'
import { request, websocketURL, audioURL } from '../lib/api'
import { RoomClock } from '../lib/sync'

const SESSION_KEY = 'musicmirror.session.v2'

function storedSession() {
  try {
    const saved = JSON.parse(sessionStorage.getItem(SESSION_KEY))
    return saved?.token && saved?.memberId ? saved : null
  } catch {
    return null
  }
}

function persist(session) {
  try {
    if (session) sessionStorage.setItem(SESSION_KEY, JSON.stringify(session))
    else sessionStorage.removeItem(SESSION_KEY)
  } catch {
    /* The current session still works when browser storage is disabled. */
  }
}

export function useRoom() {
  const [session, setSession] = useState(storedSession)
  const [room, setRoom] = useState(null)
  const [songs, setSongs] = useState([])
  const [status, setStatus] = useState(session ? 'connecting' : 'idle')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const socket = useRef(null)
  const currentRoom = useRef(null)
  const clock = useRef(new RoomClock())
  const refreshSequence = useRef(0)
  const activeToken = useRef(session?.token)
  const invalidatePlaylist = useCallback(() => {
    refreshSequence.current += 1
  }, [])

  const forget = useCallback((message = '') => {
    persist(null)
    activeToken.current = null
    setSession(null)
    setRoom(null)
    currentRoom.current = null
    setSongs([])
    setStatus('idle')
    setError(message)
  }, [])

  const refreshSongs = useCallback(async (token, signal) => {
    const sequence = ++refreshSequence.current
    const result = await request('/api/songs', { token, signal })
    if (
      sequence === refreshSequence.current &&
      token === activeToken.current &&
      !signal?.aborted
    ) {
      setSongs(
        result.songs.map((song) => ({ ...song, url: audioURL(song.url) })),
      )
    }
  }, [])

  useEffect(() => {
    if (!session) return
    activeToken.current = session.token
    const controller = new AbortController()
    const timers = new Set()
    let ws
    let stopped = false
    let attempts = 0
    let lastMessage = performance.now()
    const later = (fn, delay) => {
      const id = setTimeout(() => {
        timers.delete(id)
        fn()
      }, delay)
      timers.add(id)
      return id
    }
    const fail = (error) => {
      if (stopped || error.name === 'AbortError') return
      if (error.status === 401)
        forget(
          'Your room session ended. Create or join a room to keep listening.',
        )
      else setError(error.message)
    }
    const applySnapshot = (snapshot) => {
      if (
        stopped ||
        (currentRoom.current &&
          snapshot.playback.revision < currentRoom.current.playback.revision)
      )
        return
      currentRoom.current = snapshot
      setRoom(snapshot)
    }
    const playlist = () =>
      refreshSongs(session.token, controller.signal).catch(fail)
    const connect = () => {
      if (stopped) return
      let authenticated = false
      setStatus('connecting')
      ws = new WebSocket(websocketURL())
      const peer = ws
      socket.current = ws
      const handshake = later(() => {
        if (!authenticated) peer.close()
      }, 15000)
      ws.onopen = () =>
        peer.send(JSON.stringify({ type: 'auth', token: session.token }))
      ws.onmessage = ({ data }) => {
        if (stopped || ws !== peer) return
        lastMessage = performance.now()
        let event
        try {
          event = JSON.parse(data)
        } catch {
          return
        }
        if (event.type === 'snapshot') {
          if (!authenticated) {
            authenticated = true
            attempts = 0
            clearTimeout(handshake)
            timers.delete(handshake)
            setError('')
            playlist()
            for (const delay of [0, 250, 750, 1500])
              later(() => {
                if (peer.readyState === WebSocket.OPEN)
                  peer.send(
                    JSON.stringify({
                      type: 'ping',
                      clientTime: performance.now(),
                    }),
                  )
              }, delay)
          }
          setStatus('connected')
          applySnapshot(event.room)
          if (event.error) setError(event.error)
        } else if (event.type === 'pong') {
          clock.current.observe(event.serverTime, event.clientTime)
        } else if (event.type === 'playlist') playlist()
        else if (event.type === 'error') setError(event.error)
        else if (event.type === 'expired') forget(event.error)
      }
      ws.onerror = () => {
        /* onclose owns the bounded reconnect loop. */
      }
      ws.onclose = (event) => {
        clearTimeout(handshake)
        timers.delete(handshake)
        if (stopped || ws !== peer) return
        setStatus('connecting')
        if (event.code === 1008) {
          // Check whether the room expired or this tab's connection was replaced.
          request('/api/session', {
            token: session.token,
            signal: controller.signal,
          })
            .then(() => {
              if (!stopped)
                forget(
                  'This session was opened elsewhere. Please rejoin the room.',
                )
            })
            .catch(fail)
          return
        }
        later(
          connect,
          Math.min(10000, 500 * 2 ** attempts++) + Math.random() * 250,
        )
      }
    }
    const sent = performance.now()
    request('/api/session', { token: session.token, signal: controller.signal })
      .then((result) => {
        if (stopped) return
        clock.current.seed(result.room.serverTime, sent)
        applySnapshot(result.room)
        connect()
      })
      .catch((error) => {
        fail(error)
        if (!stopped && error.status !== 401) later(connect, 2000)
      })
    const heartbeat = setInterval(() => {
      if (ws?.readyState === WebSocket.OPEN) {
        if (performance.now() - lastMessage > 20000) ws.close()
        else
          ws.send(
            JSON.stringify({ type: 'ping', clientTime: performance.now() }),
          )
      }
    }, 10000)
    const wake = () => {
      if (document.visibilityState !== 'visible') return
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'ping', clientTime: performance.now() }))
        ws.send(JSON.stringify({ type: 'state' }))
      }
    }
    document.addEventListener('visibilitychange', wake)
    return () => {
      stopped = true
      controller.abort()
      clearInterval(heartbeat)
      timers.forEach(clearTimeout)
      document.removeEventListener('visibilitychange', wake)
      if (ws) {
        ws.onclose = null
        ws.close()
      }
      socket.current = null
      invalidatePlaylist()
    }
  }, [session, forget, refreshSongs, invalidatePlaylist])

  const enter = useCallback(async (code) => {
    setBusy(true)
    setError('')
    try {
      const result = await request(
        code ? `/api/rooms/${encodeURIComponent(code)}/join` : '/api/rooms',
        { method: 'POST', body: {} },
      )
      const saved = { token: result.token, memberId: result.memberId }
      clock.current = new RoomClock()
      currentRoom.current = null
      persist(saved)
      activeToken.current = saved.token
      setSession(saved)
    } catch (error) {
      setError(error.message)
    } finally {
      setBusy(false)
    }
  }, [])

  const send = useCallback((command) => {
    if (socket.current?.readyState !== WebSocket.OPEN) {
      setError('Reconnecting to your room. Please try again when connected.')
      return false
    }
    socket.current.send(
      JSON.stringify({
        ...command,
        revision: currentRoom.current?.playback.revision ?? 0,
      }),
    )
    return true
  }, [])

  const freshState = useCallback(async () => {
    if (!session) return null
    const result = await request('/api/session', { token: session.token })
    if (activeToken.current !== session.token) return null
    if (
      result.room.playback.revision >=
      (currentRoom.current?.playback.revision ?? 0)
    ) {
      currentRoom.current = result.room
      setRoom(result.room)
    }
    return currentRoom.current
  }, [session])

  const upload = useCallback(
    async (file) => {
      if (!session) return
      const body = new FormData()
      body.append('song', file)
      await request('/api/songs', {
        method: 'POST',
        token: session.token,
        body,
      })
      await refreshSongs(session.token)
    },
    [session, refreshSongs],
  )

  const refreshPlaylist = useCallback(async () => {
    if (!session) return
    try {
      await refreshSongs(session.token)
    } catch (error) {
      setError(error.message)
    }
  }, [session, refreshSongs])

  const leave = useCallback(async () => {
    if (!session) return
    try {
      await request('/api/session', {
        method: 'DELETE',
        token: session.token,
        signal: AbortSignal.timeout(5000),
      })
    } catch {
      /* Transport disconnect and expiry also remove inactive members. */
    }
    forget()
  }, [session, forget])

  return {
    room,
    songs,
    session,
    status,
    error,
    setError,
    busy,
    enter,
    send,
    freshState,
    upload,
    refreshPlaylist,
    leave,
    clock,
  }
}
