package catalog

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"time"

	"github.com/lib/pq"
)

//go:embed schema.sql
var Schema string

type Postgres struct{ db *sql.DB }

func OpenPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(time.Minute)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &Postgres{db: db}, nil
}
func (p *Postgres) Migrate(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx, Schema)
	return err
}
func (p *Postgres) Close() error                   { return p.db.Close() }
func (p *Postgres) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }

const columns = "id, room_id, name, object_key, content_type, size, created_at, expires_at, ready"

func scanSong(row interface{ Scan(...any) error }) (Song, error) {
	var song Song
	err := row.Scan(&song.ID, &song.Room, &song.Name, &song.ObjectKey, &song.ContentType, &song.Size, &song.CreatedAt, &song.ExpiresAt, &song.Ready)
	if errors.Is(err, sql.ErrNoRows) {
		return Song{}, ErrNotFound
	}
	song.Default = song.Room == ""
	return song, err
}
func (p *Postgres) Create(ctx context.Context, s Song) error {
	_, err := p.db.ExecContext(ctx, `INSERT INTO musicmirror_songs (`+columns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, s.ID, s.Room, s.Name, s.ObjectKey, s.ContentType, s.Size, s.CreatedAt, s.ExpiresAt, s.Ready)
	return err
}
func (p *Postgres) MarkReady(ctx context.Context, id string) error {
	res, err := p.db.ExecContext(ctx, `UPDATE musicmirror_songs SET ready=TRUE, expires_at=CASE WHEN room_id='' THEN NULL ELSE created_at + INTERVAL '6 hours' END WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}
func (p *Postgres) Get(ctx context.Context, id string) (Song, error) {
	return scanSong(p.db.QueryRowContext(ctx, `SELECT `+columns+` FROM musicmirror_songs WHERE id=$1`, id))
}
func querySongs(ctx context.Context, db *sql.DB, query string, args ...any) ([]Song, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	songs := []Song{}
	for rows.Next() {
		song, err := scanSong(rows)
		if err != nil {
			return nil, err
		}
		songs = append(songs, song)
	}
	return songs, rows.Err()
}
func (p *Postgres) List(ctx context.Context, room string, now time.Time) ([]Song, error) {
	return querySongs(ctx, p.db, `SELECT `+columns+` FROM musicmirror_songs WHERE ready AND (room_id='' OR room_id=$1) AND (expires_at IS NULL OR expires_at>$2) ORDER BY (room_id='') DESC,created_at,id`, room, now)
}
func (p *Postgres) TotalSize(ctx context.Context) (int64, error) {
	var total int64
	err := p.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size),0) FROM musicmirror_songs`).Scan(&total)
	return total, err
}
func (p *Postgres) ExpireRooms(ctx context.Context, codes []string, now time.Time) error {
	if len(codes) == 0 {
		return nil
	}
	_, err := p.db.ExecContext(ctx, `UPDATE musicmirror_songs SET expires_at=$1 WHERE room_id<>'' AND room_id=ANY($2)`, now, pq.Array(codes))
	return err
}
func (p *Postgres) Expired(ctx context.Context, now time.Time) ([]Song, error) {
	return querySongs(ctx, p.db, `SELECT `+columns+` FROM musicmirror_songs WHERE expires_at<=$1 ORDER BY expires_at LIMIT 200`, now)
}
func (p *Postgres) Delete(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM musicmirror_songs WHERE id=$1`, id)
	return err
}
