# MusicMirror architecture

## Scope and deployment

One Go process owns active rooms. React is a static site. Supabase PostgreSQL stores
song metadata; a private Supabase Storage bucket stores audio. Browser playback
uses signed object URLs, so audio bandwidth does not pass through Render.
The free hosting target is one Render web service and one Render static site.
Free service restarts end active rooms; uploaded files remain until expiry cleanup.
Supabase projects paused for inactivity must be resumed from its dashboard.

Local development uses an in-memory metadata repository and local files, with no
credentials. Production refuses this mode. Interfaces separate persistence and
object storage from HTTP, WebSocket, and playback logic.

## Room and session lifecycle

The server generates six-character room codes, random display names, and 256-bit
session tokens. A room code is an invitation: anyone with it can join. Tokens are
stored in sessionStorage and authorize HTTP requests. WebSockets authenticate in
their first message, never in a URL. The server stores token hashes in memory.
Sessions survive socket disconnects for up to two minutes. A new connection replaces the old
connection for that session. Rooms expire after six hours, or after two minutes
with no connected members. Explicit leave revokes the session.

Limits bound rooms, members, files, storage, message sizes, command frequency,
and disconnected sessions. Origins are allowlisted. There are no user accounts.

## Playback semantics

After a track is selected the room timeline runs independently of any listener.
The authoritative state is (song ID, position in seconds, server timestamp in
milliseconds, monotonically increasing revision). Position at time t is
max(0, reference position + (t - reference timestamp)/1000).

- Selecting a song changes the room track and starts at zero.
- Seeking changes the room timeline for all listeners. A locally paused listener
  stays paused but receives the new reference state.
- Pause and resume never change the room timeline. Resume fetches current state
  and aligns before playing. A locally paused listener stays paused on track changes.
- Clients estimate server clock offset using ping round trips and favor samples
  with lower latency. State broadcasts every two seconds refresh the reference.
- Small drift is corrected by bounded playback-rate adjustment; larger drift is
  corrected by seeking only when the target is buffered with at least 750 ms of
  playable headroom (or through the end of the track). Until then, use bounded
  rate correction rather than restart a range download. Explicit user seeks
  still jump immediately. Suspended tabs and reconnects request fresh state.
- Commands include the revision observed by the sender. Stale commands return
  the current state instead of overriding a more recent operation.
- Track-end advances use the same revision check so simultaneous clients can
  advance only once. Autoplay restrictions show an explicit resume control.

Browser scheduling, buffering, and clock asymmetry prevent a universal precision
guarantee. Test clock math deterministically, and validate real browsers separately.

## Transport contract

HTTP JSON errors use {"error": "safe public message"}. All responses are no-store.

| Route | Purpose |
| --- | --- |
| GET /healthz | Process liveness |
| GET /readyz | Metadata/storage configuration readiness |
| POST /api/rooms | Create room and session |
| POST /api/rooms/{code}/join | Join existing room |
| GET /api/session | Resume session and fetch room snapshot |
| DELETE /api/session | Leave room and revoke session |
| GET /api/songs | List default and room songs with signed playback URLs |
| POST /api/songs | Upload multipart field `song` to current room |
| GET /media/{key} | Signed local-development audio URL with byte-range support |
| GET /api/ws | Native WebSocket |

First WS message: {"type":"auth","token":"..."}. Server sends a `snapshot`.
Client messages: `ping` (clientTime), `state`, `select` (songId, revision),
`seek` (position, revision). Server messages: `snapshot` (room), `pong`
(clientTime, serverTime), `playlist`, `error` (error). Snapshot room members
contain id, name, connected; playback contains songId, position, updatedAt,
revision; every room snapshot includes serverTime.

## Persistence and cleanup

Room uploads expire after six hours, and are deleted early when a room expires.
Default songs have no room or expiry. Uploads are bounded and checked by content,
not filename or browser MIME alone. Random object names prevent collisions.
Metadata is recorded before object upload so failed cleanup can be retried.
Incomplete rows are hidden from playlists. Cleanup deletes the object first,
then metadata; failures leave a retryable row. Expiry cleanup also runs after a
restart. Existing Cloudinary audio is not copied automatically.

## Verification

Before every commit: `make check-backend` (format, vet, race-enabled tests).
CI also runs against real PostgreSQL and checks frontend lint, unit tests, build,
and browser smoke tests. Storage adapter tests use a local fake HTTP server, never
live credentials. Fake time tests exercise timeline ordering and room expiry.

