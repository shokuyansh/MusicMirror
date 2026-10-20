# MusicMirror

A shared listening room for friends. Create a room, send its invite link, upload
music, and listen together. No accounts required.

- **Go backend:** `net/http`, `database/sql` with `github.com/lib/pq`, and native
  WebSockets with `github.com/coder/websocket`.
- **React + Vite frontend:** accessible keyboard controls, responsive layout,
  reconnect handling, and clear local-pause/autoplay states.
- **Supabase:** private audio storage and PostgreSQL metadata. Browsers stream
  using temporary URLs; storage credentials stay on the server.

## Playback behavior

Selecting a track or seeking changes playback for the room. Pausing and volume
changes affect only you. Press play to fetch the latest room timeline and resume
in sync. A periodic clock exchange and drift correction keep active listeners
aligned. Late joins and reconnects use the server's timeline.

Rooms accept up to 20 listeners and expire after six hours, or after two minutes
with nobody connected. Disconnected sessions can rejoin within two minutes.
Room uploads are temporary; default tracks persist. A backend restart ends live
rooms. Browser buffering and background throttling can affect sync precision.

## Run locally

Requires **Go 1.25+** and **Node 22.12+**. No Supabase account is needed locally.

In one terminal:

```sh
cd server
go run ./cmd/musicmirror
```

In another:

```sh
cd client
npm ci
npm run dev
```

Open `http://localhost:5173`. Vite proxies API, WebSocket, and local audio requests
to port 3001. Local mode uses in-memory metadata and files under `server/data`;
playlists and URL grants reset on restart. Production requires all Supabase
settings and refuses this temporary local mode.

Environment variable names are listed in [server/.env.example](server/.env.example)
and [client/.env.example](client/.env.example). Vite reads frontend `.env` files.
Go reads the process environment; it does **not** load `.env` automatically.
When using a backend `.env`, quote its secret values and load it in your shell
before running Go:

```sh
set -a
. ./.env
set +a
go run ./cmd/musicmirror
```

## Checks

From the project root:

```sh
make hooks             # Install the backend pre-commit check for this checkout
make check-backend     # gofmt, go vet, race-enabled unit and integration tests
make check-frontend    # ESLint, formatting, sync unit tests, production build
```

To include real PostgreSQL tests, set `TEST_DATABASE_URL` to a **dedicated test
database** before running `make check-backend`. The suite creates the metadata
table and round-trips test rows. Without that variable only the PostgreSQL test
is skipped; HTTP, WebSocket, object storage and room tests still run locally.
CI always supplies a PostgreSQL service.

For browser tests:

```sh
cd client
npx playwright install chromium
npm run test:e2e
```

Playwright starts the local Go and Vite servers. Tests cover two listeners,
seeking, local pause/resume, late joins, reloads, reconnects, multiple uploads,
autoplay rejection, track advancement, simulated network latency, and a mobile
viewport. Backend tests also cover authorization, origin checks, concurrent
commands, quota races, expiry, slow peers, and failed cleanup retries.

## Deploy online for free

Follow [the deployment guide](docs/deployment.md). It includes Supabase setup,
exact Render environment variables, optional default-song import, free-plan
limits, and deployment checks. [render.yaml](render.yaml) defines a free backend
and static frontend; it provisions no paid database or disk.

The app runs on free plans **within provider quotas**. Supabase inactivity pauses
require a manual dashboard resume. Existing Cloudinary files are not migrated
automatically. Nothing is deployed by running the local test suite.

See [architecture and transport contract](docs/architecture.md) for implementation
boundaries and playback semantics.
