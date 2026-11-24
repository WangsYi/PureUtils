package pureshot

import (
	"database/sql"
	"fmt"

	"pureutils/pkg/purekit/storage"
)

const shotSchema = `
CREATE TABLE IF NOT EXISTS shot_entry (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    image_path       TEXT    NOT NULL,
    saved_path       TEXT,
    annotation_json  TEXT,
    width            INTEGER NOT NULL DEFAULT 0,
    height           INTEGER NOT NULL DEFAULT 0,
    target           TEXT    NOT NULL DEFAULT 'clipboard',
    created_utc      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_shot_created ON shot_entry(created_utc DESC);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

// ShotRecord is one screenshot row stored in PureShot's isolated shots.db.
type ShotRecord struct {
	ID             int64
	ImagePath      string
	SavedPath      string
	AnnotationJSON string
	Width          int
	Height         int
	Target         string
	CreatedUTC     int64
}

// Repo manages PureShot's isolated shot history SQLite database.
type Repo struct {
	db *storage.DB
}

// OpenRepo opens shots.db inside store.
func OpenRepo(store storage.PluginStorage) (*Repo, error) {
	if store == nil {
		return nil, fmt.Errorf("pureshot: nil storage")
	}
	db, err := store.OpenDB("shots.db", storage.WithSchema(shotSchema))
	if err != nil {
		return nil, err
	}
	return &Repo{db: db}, nil
}

// Close checkpoints and closes shots.db.
func (r *Repo) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	err1 := r.db.Checkpoint()
	err2 := r.db.Close()
	r.db = nil
	if err1 != nil {
		return err1
	}
	return err2
}

// InsertShot records one completed screenshot.
func (r *Repo) InsertShot(s ShotRecord) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("pureshot: repo closed")
	}
	res, err := r.db.SQL().Exec(`
		INSERT INTO shot_entry (image_path, saved_path, annotation_json, width, height, target, created_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.ImagePath, nullIfEmpty(s.SavedPath), nullIfEmpty(s.AnnotationJSON), s.Width, s.Height, s.Target, s.CreatedUTC)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RecentShots lists recent screenshots newest first.
func (r *Repo) RecentShots(limit int) ([]ShotRecord, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.SQL().Query(`SELECT id, image_path, saved_path, annotation_json, width, height, target, created_utc FROM shot_entry ORDER BY created_utc DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ShotRecord
	for rows.Next() {
		var s ShotRecord
		var saved, ann sql.NullString
		if err := rows.Scan(&s.ID, &s.ImagePath, &saved, &ann, &s.Width, &s.Height, &s.Target, &s.CreatedUTC); err != nil {
			return nil, err
		}
		s.SavedPath = saved.String
		s.AnnotationJSON = ann.String
		out = append(out, s)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
