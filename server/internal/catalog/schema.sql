CREATE TABLE IF NOT EXISTS musicmirror_songs (
    id TEXT PRIMARY KEY,
    room_id TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    object_key TEXT NOT NULL UNIQUE,
    content_type TEXT NOT NULL,
    size BIGINT NOT NULL CHECK (size > 0),
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ,
    ready BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS musicmirror_songs_room_idx ON musicmirror_songs(room_id);
CREATE INDEX IF NOT EXISTS musicmirror_songs_expiry_idx ON musicmirror_songs(expires_at);
-- No browser access to metadata. Go uses the server-only database connection.
ALTER TABLE musicmirror_songs ENABLE ROW LEVEL SECURITY;
