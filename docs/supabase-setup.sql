-- Run once in the SQL Editor of your Supabase Free project.
-- Uploads use the Go backend's server-only credentials. No public write policies.
INSERT INTO storage.buckets (id,name,public,file_size_limit,allowed_mime_types)
VALUES ('music','music',FALSE,20971520,ARRAY['audio/mpeg','audio/wav','audio/ogg'])
ON CONFLICT (id) DO NOTHING;

-- The Go backend creates its metadata table and indexes on startup.
-- If you prefer to provision tables here, run server/internal/catalog/schema.sql.
-- Do not add anon/authenticated policies to musicmirror_songs or storage.objects.
