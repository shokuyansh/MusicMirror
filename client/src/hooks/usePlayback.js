import { useCallback, useEffect, useRef, useState } from 'react'
import { correction, positionAt } from '../lib/sync'

export function usePlayback({
  room,
  songs,
  clock,
  freshState,
  send,
  setError,
  status,
}) {
  const audio = useRef(null)
  const live = useRef({ room, songs, status })
  const desired = useRef(true)
  const generation = useRef(0)
  const advancing = useRef(null)
  const [playing, setPlaying] = useState(false)
  const [paused, setPaused] = useState(false)
  const [blocked, setBlocked] = useState(false)
  const [loading, setLoading] = useState(false)
  const [currentTime, setCurrentTime] = useState(0)
  const [duration, setDuration] = useState(0)
  const [volume, setVolume] = useState(0.8)
  const currentSong = songs.find((song) => song.id === room?.playback.songId)
  const songId = currentSong?.id
  const songURL = currentSong?.url
  const invalidate = useCallback(() => {
    generation.current += 1
  }, [])

  useEffect(() => {
    live.current = { room, songs, status }
  }, [room, songs, status])

  const align = useCallback(
    (force = false, snapshot = live.current.room) => {
      const element = audio.current
      if (!element || !snapshot?.playback.songId || element.readyState < 1)
        return
      const target = positionAt(
        snapshot.playback,
        clock.current.now(),
        element.duration,
      )
      const adjustment = correction(element.currentTime, target)
      if (
        (force || adjustment.seek !== null) &&
        Math.abs(element.currentTime - target) > 0.01
      )
        element.currentTime = target
      element.playbackRate = desired.current ? adjustment.rate : 1
    },
    [clock],
  )

  const start = useCallback(async () => {
    const element = audio.current
    if (
      !element ||
      !desired.current ||
      element.readyState < 2 ||
      !element.paused
    )
      return
    const attempt = generation.current
    try {
      await element.play()
      if (!desired.current || attempt !== generation.current) return
      setPlaying(true)
      setBlocked(false)
    } catch (error) {
      if (error.name === 'AbortError' || attempt !== generation.current) return
      desired.current = false
      setPlaying(false)
      setPaused(true)
      if (error.name === 'NotAllowedError') setBlocked(true)
      else
        setError('This song could not play. Try another track or resume again.')
    }
  }, [setError])

  useEffect(() => {
    const element = audio.current
    desired.current = true
    invalidate()
    setPaused(false)
    setBlocked(false)
    advancing.current = null
    return () => {
      invalidate()
      desired.current = false
      if (element) {
        element.pause()
        element.removeAttribute('src')
        element.load()
      }
    }
  }, [room?.code, invalidate])

  useEffect(() => {
    const element = audio.current
    if (!element || !songId || !songURL) return
    generation.current++
    setLoading(true)
    setPlaying(false)
    setCurrentTime(0)
    setDuration(0)
    element.src = songURL
    element.load()
    const loaded = () => {
      setDuration(Number.isFinite(element.duration) ? element.duration : 0)
      align(true)
    }
    const ready = () => {
      setLoading(false)
      if (element.paused) align()
      start()
    }
    const stalled = () => setLoading(true)
    const failed = () => {
      desired.current = false
      setLoading(false)
      setPlaying(false)
      setPaused(true)
      setError(
        'Audio could not load. Press play to retry, or choose another song.',
      )
    }
    const ended = () => {
      setPlaying(false)
      const state = live.current
      if (!desired.current || state.status !== 'connected' || !state.room)
        return
      const revision = state.room.playback.revision
      if (advancing.current === revision) return
      const index = state.songs.findIndex(
        (song) => song.id === state.room.playback.songId,
      )
      if (index < 0 || state.songs.length === 0) return
      advancing.current = revision
      send({
        type: 'select',
        songId: state.songs[(index + 1) % state.songs.length].id,
      })
    }
    element.addEventListener('loadedmetadata', loaded)
    element.addEventListener('canplay', ready)
    element.addEventListener('waiting', stalled)
    element.addEventListener('error', failed)
    element.addEventListener('ended', ended)
    return () => {
      element.pause()
      element.removeEventListener('loadedmetadata', loaded)
      element.removeEventListener('canplay', ready)
      element.removeEventListener('waiting', stalled)
      element.removeEventListener('error', failed)
      element.removeEventListener('ended', ended)
    }
  }, [songId, songURL, align, start, send, setError])

  useEffect(() => {
    if (!room?.playback.songId) return
    // A room seek updates a paused listener's position without starting audio.
    align(true)
    start()
  }, [room?.playback.revision, room?.playback.songId, align, start])

  useEffect(() => {
    const interval = setInterval(() => {
      const element = audio.current
      if (!element || !element.src) return
      setCurrentTime(element.currentTime)
      if (
        desired.current &&
        !element.paused &&
        !element.seeking &&
        element.readyState >= 3
      )
        align()
    }, 250)
    return () => clearInterval(interval)
  }, [align])

  useEffect(() => {
    if (audio.current) audio.current.volume = volume
  }, [volume, room?.code])

  const toggle = useCallback(async () => {
    const element = audio.current
    if (!element || !live.current.room?.playback.songId) return
    if (desired.current && !element.paused) {
      desired.current = false
      generation.current++
      element.pause()
      element.playbackRate = 1
      setPlaying(false)
      setPaused(true)
      return
    }
    if (live.current.status !== 'connected') {
      setError('Reconnect to the room before resuming.')
      return
    }
    const attempt = ++generation.current
    setLoading(true)
    try {
      const state = await freshState()
      if (attempt !== generation.current || !state) return
      desired.current = true
      setPaused(false)
      setBlocked(false)
      if (element.error) element.load()
      // If the track changed during the request, its load effect aligns it.
      if (state.playback.songId === live.current.room?.playback.songId) {
        align(true, state)
        await start()
      }
    } catch (error) {
      setError(error.message)
    } finally {
      if (attempt === generation.current) setLoading(false)
    }
  }, [freshState, align, start, setError])

  const select = useCallback(
    (songId) => {
      if (!send({ type: 'select', songId })) return
      desired.current = true
      generation.current++
      setPaused(false)
      setBlocked(false)
    },
    [send],
  )

  const skip = useCallback(
    (direction) => {
      const state = live.current
      if (state.songs.length === 0) return
      const index = state.songs.findIndex(
        (song) => song.id === state.room?.playback.songId,
      )
      const next =
        index < 0
          ? 0
          : (index + direction + state.songs.length) % state.songs.length
      select(state.songs[next].id)
    },
    [select],
  )

  const seek = useCallback(
    (value) => {
      send({ type: 'seek', position: value })
    },
    [send],
  )

  return {
    audio,
    currentSong,
    playing,
    paused,
    blocked,
    loading,
    currentTime,
    duration,
    volume,
    setVolume,
    toggle,
    select,
    skip,
    seek,
  }
}
