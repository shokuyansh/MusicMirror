# Free online deployment

## 1. Prepare Supabase

Create one **Free** Supabase project. Save its database password securely.
In the SQL Editor, run [supabase-setup.sql](supabase-setup.sql) to create a private
`music` bucket. The backend creates its metadata table and indexes automatically.
The bucket permits MP3, WAV and OGG uploads, up to 20 MB each.

Collect these values for **backend environment variables only**:

| Variable | Where to find it |
| --- | --- |
| `SUPABASE_URL` | Project URL in project settings / API |
| `SUPABASE_SECRET_KEY` | Server secret key (`sb_secret_...`), or legacy `service_role` key |
| `DATABASE_URL` | Connect → Session pooler → PostgreSQL URI, using your database password |

Use the IPv4-compatible **session pooler on port 5432**, not the direct
IPv6 connection. Append `?sslmode=require` (or `&sslmode=require` if the URI already
has a query). URL-encode special characters in the password. Never put these
values in `VITE_*` variables, source control, logs, or chat.

The database connection creates a `musicmirror_songs` table with row-level
security enabled and no browser access policies. The backend is responsible for
authorization. The private bucket also needs no public upload policies.

## 2. Prepare Render

The included [render.yaml](../render.yaml) defines a **Free** Go web service and
a free static React site, with deployments gated on passing CI. Push the reviewed
commits to your repository when ready, then create a Render Blueprint from it.
You can alternatively configure two services manually using the same settings.

Choose your service names first so both public URLs are known. If Render assigns
different hostnames, copy the actual URLs from its dashboard and update:

| Service | Variable | Example |
| --- | --- | --- |
| Backend | `ALLOWED_ORIGINS` | `https://musicmirror-web.onrender.com` |
| Frontend | `VITE_API_URL` | `https://musicmirror-api.onrender.com` |

Use exact origins without trailing slashes or paths. Multiple frontend origins
can be comma-separated in `ALLOWED_ORIGINS`. Production requires HTTPS.
Set the three Supabase variables above on the backend. Keep `SUPABASE_BUCKET=music`.

Manual backend settings: root directory `server`, runtime Go, **Free** instance,
build `go build -trimpath -o musicmirror ./cmd/musicmirror`, start `./musicmirror`,
`APP_ENV=production`, `TRUST_PROXY=true`, health check `/healthz`.
The server binds Render's `PORT`. `/readyz` checks database availability.
`TRUST_PROXY=true` is only appropriate behind a trusted proxy that appends the
real client address to `X-Forwarded-For`; turn it off for direct deployments.

Manual frontend settings: root `client`, runtime Static Site, Node 22,
build `npm ci && npm run build`, publish `dist`, rewrite `/*` to `/index.html`.
Vite embeds `VITE_API_URL` during the build; redeploy after changing it.

There is no Render PostgreSQL instance, paid disk, worker, or other paid resource
in this setup. Use the providers' Free plans. Render can bill overages if a
payment method is attached; without one it suspends affected services instead.

## 3. Verify the deployment

1. Open backend `/healthz` and `/readyz`: both should return HTTP 200.
2. Open the frontend, create a room, and upload a small audio file.
3. Join from another browser/device using the invite link. Both playlists should update.
4. Select a song, seek, locally pause one listener, then resume it. Check that the
   other listener continues and the resumed listener catches up.
5. Refresh a listener: it should rejoin without adding another member.

Signed URLs expire after 24 hours; rooms last at most six hours. Uploads are
deleted when their room expires, or at their own six-hour expiry. Room expiry
also occurs two minutes after the last listener disconnects. A separate cleanup
pass retries failed deletions and runs at startup and every 30 seconds. If a free
server is asleep, cleanup runs when it wakes.

Backend restarts end active rooms because their live timeline and sessions are
in memory. Audio remains in Supabase until cleanup removes expired files.
Reopen the app and create a new room after a restart. Default songs persist.

## Optional default playlist

Supply your own permitted audio files; existing Cloudinary songs are not migrated
automatically. On your computer, load the backend environment, then run from `server`:

```sh
go run ./cmd/import-song /path/to/song.mp3
```

This operator command writes a permanent default track to the same private bucket
and database. It is not a browser endpoint. Up to 20 default tracks count toward
the shared app storage quota. Room playlists contain defaults followed by uploads.

## Free tier limits

The app defaults to 20 MB/file, 20 uploads/room, 20 members/room, 100 active rooms,
and 500 MiB total tracked audio. Uploads stop at the quota. These limits leave
headroom under Supabase's file storage allocation, but do not impose a hard limit
on storage-provider bandwidth. Each listener downloads audio separately.

As checked on 2026-10-20, Supabase Free includes 1 GB file storage, 500 MB database,
5 GB uncached and 5 GB cached egress. Free projects can pause after low database
activity over seven days and must be resumed from the dashboard. The current
dashboard restore window is one year after pausing. Export your database and
audio separately if you want backups; database dumps do not include audio files.

Render Free sleeps after 15 minutes without inbound traffic and can take about
a minute to wake. The frontend handles delayed startup and reconnects, but this
setup does not promise continuous availability or zero cost above provider limits.

Sources: [Supabase pricing](https://supabase.com/pricing),
[project pausing](https://supabase.com/docs/guides/platform/free-project-pausing),
[database connections](https://supabase.com/docs/guides/database/connecting-to-postgres),
[Render Free](https://render.com/docs/free),
[Render billing FAQ](https://render.com/docs/faq).
