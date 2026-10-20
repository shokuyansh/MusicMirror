import { useRef, useState } from 'react'
import Icon from './Icon'

const MAX_UPLOAD_MB = Number(import.meta.env.VITE_MAX_UPLOAD_MB) || 20

export default function UploadCard({ upload, disabled, setError }) {
  const [file, setFile] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [dragging, setDragging] = useState(false)
  const [message, setMessage] = useState('')
  const input = useRef(null)
  const choose = (selected) => {
    setMessage('')
    if (!selected) return
    if (
      !/\.(mp3|wav|ogg)$/i.test(selected.name) ||
      selected.size > MAX_UPLOAD_MB * 1024 * 1024
    ) {
      setError(`Choose an MP3, WAV or OGG file up to ${MAX_UPLOAD_MB} MB.`)
      return
    }
    setFile(selected)
  }
  const submit = async (event) => {
    event.preventDefault()
    if (!file || uploading) return
    setUploading(true)
    setMessage('')
    try {
      await upload(file)
      setFile(null)
      if (input.current) input.current.value = ''
      setMessage('Added to the room playlist.')
    } catch (error) {
      setError(error.message)
    } finally {
      setUploading(false)
    }
  }
  return (
    <section className="panel upload-panel">
      <div className="panel-heading">
        <Icon name="upload" />
        <h2>Bring your music</h2>
      </div>
      <p className="muted">The next great track is yours.</p>
      <form onSubmit={submit}>
        <label
          className={`dropzone ${dragging ? 'dragging' : ''}`}
          onDragOver={(event) => {
            event.preventDefault()
            if (!disabled && !uploading) setDragging(true)
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(event) => {
            event.preventDefault()
            setDragging(false)
            if (!disabled && !uploading) choose(event.dataTransfer.files[0])
          }}
        >
          <input
            ref={input}
            className="sr-only"
            type="file"
            aria-label="Choose audio file"
            accept=".mp3,.wav,.ogg,audio/mpeg,audio/wav,audio/ogg"
            onChange={(event) => choose(event.target.files[0])}
            disabled={disabled || uploading}
          />
          <span className="drop-icon">
            <Icon name={file ? 'music' : 'upload'} size={23} />
          </span>
          <strong>{file ? file.name : 'Choose a track'}</strong>
          <span>
            {file
              ? `${(file.size / 1024 / 1024).toFixed(1)} MB · ready to upload`
              : 'or drop it right here'}
          </span>
        </label>
        <div className="file-hint">
          MP3, WAV, OGG <span>UP TO {MAX_UPLOAD_MB} MB</span>
        </div>
        <button
          className="button secondary upload-button"
          type="submit"
          disabled={!file || uploading || disabled}
        >
          {uploading ? (
            <>
              <span className="spinner" /> Adding your track…
            </>
          ) : (
            <>
              <Icon name="upload" size={17} /> Add to playlist
            </>
          )}
        </button>
      </form>
      <p className="upload-message" role="status">
        {message}
      </p>
    </section>
  )
}
