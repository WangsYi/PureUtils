package pureclip

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/storage"
)

// ErrLegacyReadOnly is returned when attempting to mutate or delete a record
// read from a read-only legacy database (pureutils.db / shotclip.db).
var ErrLegacyReadOnly = errors.New("pureclip: legacy history record is read-only")

const clipSchema = `
CREATE TABLE IF NOT EXISTS clipboard_entry (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    kind           INTEGER NOT NULL,
    preview        TEXT    NOT NULL,
    body           TEXT,
    hash           TEXT    NOT NULL UNIQUE,
    byte_size      INTEGER NOT NULL DEFAULT 0,
    width          INTEGER NOT NULL DEFAULT 0,
    height         INTEGER NOT NULL DEFAULT 0,
    image_path     TEXT,
    payload_json   TEXT,
    created_utc    INTEGER NOT NULL,
    last_used_utc  INTEGER NOT NULL,
    use_count      INTEGER NOT NULL DEFAULT 0,
    favorite       INTEGER NOT NULL DEFAULT 0,
    source_process TEXT,
    body_truncated INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS ix_clipboard_used ON clipboard_entry(last_used_utc DESC);
CREATE INDEX IF NOT EXISTS ix_clipboard_created ON clipboard_entry(created_utc DESC);
CREATE INDEX IF NOT EXISTS ix_clipboard_kind ON clipboard_entry(kind);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

// Payload is the JSON sidecar stored with a clipboard record.
type Payload struct {
	HTML  string   `json:"html,omitempty"`
	RTF   string   `json:"rtf,omitempty"`
	Files []string `json:"files,omitempty"`
}

// EncodePayload encodes the sidecar.
func EncodePayload(p Payload) string {
	b, _ := json.Marshal(p)
	return string(b)
}

// DecodePayload decodes the sidecar, tolerating malformed rows.
func DecodePayload(s string) Payload {
	var p Payload
	if s != "" {
		_ = json.Unmarshal([]byte(s), &p)
	}
	return p
}

// Record is one row of clipboard history. Negative IDs denote read-only rows
// surfaced from a legacy pureutils.db / shotclip.db fallback database.
type Record struct {
	ID            int64
	Kind          core.ClipboardKind
	Preview       string
	Body          string
	Hash          string
	ByteSize      int
	Width         int
	Height        int
	ImagePath     string
	PayloadJSON   string
	CreatedUTC    int64
	LastUsedUTC   int64
	UseCount      int
	Favorite      bool
	SourceProcess string
	BodyTruncated bool
}

// IsLegacy reports whether this record was read from a read-only legacy database.
func (r Record) IsLegacy() bool { return r.ID < 0 }

// RestoreOutcome reports what happened when a history row was replayed.
type RestoreOutcome struct {
	Wrote        bool
	MissingFiles []string
}

// PruneResult reports a deletion.
type PruneResult struct {
	RemovedEntries int
	RemovedImages  []string
}

// IngestOutcome reports whether a clipboard row was new.
type IngestOutcome struct {
	Record       Record
	WasDuplicate bool
}

// Repo manages PureClip's isolated clip.db and an optional read-only legacy DB
// fallback without ever mutating or migrating legacy files on disk.
type Repo struct {
	db       *storage.DB
	legacyDB *sql.DB
}

// OpenRepo opens clip.db inside store and optionally attaches a read-only
// legacy database fallback.
func OpenRepo(store storage.PluginStorage, log *core.Logger) (*Repo, error) {
	if store == nil {
		return nil, fmt.Errorf("pureclip: nil storage")
	}
	db, err := store.OpenDB("clip.db", storage.WithSchema(clipSchema))
	if err != nil {
		return nil, err
	}
	var legacy *sql.DB
	if ldb, lpath, lerr := store.OpenLegacyDB(); lerr == nil {
		legacy = ldb
		if log != nil {
			log.Infof("PureClip 已附加只读旧库兼容视图: %s", lpath)
		}
	} else if !errors.Is(lerr, os.ErrNotExist) && log != nil {
		log.Warnf("PureClip 打开只读旧库失败，已忽略: %v", lerr)
	}
	return &Repo{db: db, legacyDB: legacy}, nil
}

// Close checkpoints and closes the writable DB, and closes the legacy DB
// strictly without writing or checkpointing.
func (r *Repo) Close() error {
	if r == nil {
		return nil
	}
	var firstErr error
	if r.db != nil {
		if err := r.db.Checkpoint(); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := r.db.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		r.db = nil
	}
	if r.legacyDB != nil {
		if err := r.legacyDB.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		r.legacyDB = nil
	}
	return firstErr
}

// InsertOrTouch adds a record to writable clip.db or bumps the existing hash row.
func (r *Repo) InsertOrTouch(c Record) (IngestOutcome, error) {
	tx, err := r.db.SQL().Begin()
	if err != nil {
		return IngestOutcome{}, err
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRow(`SELECT id FROM clipboard_entry WHERE hash = ?`, c.Hash).Scan(&id)
	if err == nil {
		_, err = tx.Exec(`
			UPDATE clipboard_entry
			   SET last_used_utc = ?, use_count = use_count + 1, preview = ?,
			       image_path = COALESCE(?, image_path),
			       payload_json = COALESCE(?, payload_json),
			       source_process = COALESCE(?, source_process)
			 WHERE id = ?`, c.LastUsedUTC, c.Preview, nullIfEmpty(c.ImagePath),
			nullIfEmpty(c.PayloadJSON), nullIfEmpty(c.SourceProcess), id)
		if err != nil {
			return IngestOutcome{}, err
		}
		stored, err := getTx(tx, id)
		if err != nil {
			return IngestOutcome{}, err
		}
		if err := tx.Commit(); err != nil {
			return IngestOutcome{}, err
		}
		return IngestOutcome{Record: stored, WasDuplicate: true}, nil
	}
	if err != sql.ErrNoRows {
		return IngestOutcome{}, err
	}

	res, err := tx.Exec(`
		INSERT INTO clipboard_entry
		    (kind, preview, body, hash, byte_size, width, height, image_path, payload_json,
		     created_utc, last_used_utc, use_count, favorite, source_process, body_truncated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		int(c.Kind), c.Preview, nullIfEmpty(c.Body), c.Hash, c.ByteSize, c.Width, c.Height,
		nullIfEmpty(c.ImagePath), nullIfEmpty(c.PayloadJSON), c.CreatedUTC, c.LastUsedUTC,
		boolInt(c.Favorite), nullIfEmpty(c.SourceProcess), boolInt(c.BodyTruncated))
	if err != nil {
		return IngestOutcome{}, err
	}
	id, err = res.LastInsertId()
	if err != nil {
		return IngestOutcome{}, err
	}
	c.ID = id
	if err := tx.Commit(); err != nil {
		return IngestOutcome{}, err
	}
	return IngestOutcome{Record: c, WasDuplicate: false}, nil
}

// Search lists rows newest-used first from writable clip.db plus read-only legacy fallback.
func (r *Repo) Search(query string, kind *core.ClipboardKind, favoriteOnly *bool, limit int) ([]Record, error) {
	if limit < 1 {
		limit = 1
	}
	if limit > 2000 {
		limit = 2000
	}
	sqlStr, args := buildSearchQuery(query, kind, favoriteOnly, limit)

	out, err := runSearchQuery(r.db.SQL(), sqlStr, args, false)
	if err != nil {
		return nil, err
	}
	if r.legacyDB != nil {
		seen := make(map[string]bool, len(out))
		for _, rec := range out {
			seen[rec.Hash] = true
		}
		legacyRows, lerr := runSearchQuery(r.legacyDB, sqlStr, args, true)
		if lerr == nil {
			for _, rec := range legacyRows {
				if seen[rec.Hash] {
					continue
				}
				seen[rec.Hash] = true
				out = append(out, rec)
			}
			sort.Slice(out, func(i, j int) bool {
				if out[i].LastUsedUTC != out[j].LastUsedUTC {
					return out[i].LastUsedUTC > out[j].LastUsedUTC
				}
				return out[i].ID > out[j].ID
			})
			if len(out) > limit {
				out = out[:limit]
			}
		}
	}
	return out, nil
}

func buildSearchQuery(query string, kind *core.ClipboardKind, favoriteOnly *bool, limit int) (string, []any) {
	var b strings.Builder
	b.WriteString(`SELECT id, kind, preview, body, hash, byte_size, width, height, image_path,
		payload_json, created_utc, last_used_utc, use_count, favorite, source_process, body_truncated
		FROM clipboard_entry WHERE 1=1`)
	var args []any
	if strings.TrimSpace(query) != "" {
		b.WriteString(` AND (preview LIKE ? ESCAPE '\' OR IFNULL(body,'') LIKE ? ESCAPE '\' OR IFNULL(source_process,'') LIKE ? ESCAPE '\')`)
		like := "%" + escapeLike(strings.TrimSpace(query)) + "%"
		args = append(args, like, like, like)
	}
	if kind != nil {
		if *kind == core.KindText {
			b.WriteString(" AND kind IN (?, ?)")
			args = append(args, int(core.KindText), int(core.KindRichText))
		} else {
			b.WriteString(" AND kind = ?")
			args = append(args, int(*kind))
		}
	}
	if favoriteOnly != nil {
		b.WriteString(" AND favorite = ?")
		args = append(args, boolInt(*favoriteOnly))
	}
	b.WriteString(" ORDER BY last_used_utc DESC, id DESC LIMIT ?")
	args = append(args, limit)
	return b.String(), args
}

func runSearchQuery(db *sql.DB, sqlStr string, args []any, negateID bool) ([]Record, error) {
	rows, err := db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		if negateID && rec.ID > 0 {
			rec.ID = -rec.ID
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Get returns one record by ID (negative ID queries the read-only legacy database).
func (r *Repo) Get(id int64) (*Record, error) {
	if id < 0 {
		if r.legacyDB == nil {
			return nil, nil
		}
		row := r.legacyDB.QueryRow(`SELECT id, kind, preview, body, hash, byte_size, width, height, image_path,
			payload_json, created_utc, last_used_utc, use_count, favorite, source_process, body_truncated
			FROM clipboard_entry WHERE id = ?`, -id)
		rec, err := scanRecord(row)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, nil
		}
		if rec.ID > 0 {
			rec.ID = -rec.ID
		}
		return &rec, nil
	}

	row := r.db.SQL().QueryRow(`SELECT id, kind, preview, body, hash, byte_size, width, height, image_path,
		payload_json, created_utc, last_used_utc, use_count, favorite, source_process, body_truncated
		FROM clipboard_entry WHERE id = ?`, id)
	rec, err := scanRecord(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// SetFavorite sets the favorite flag on writable rows.
func (r *Repo) SetFavorite(id int64, favorite bool) (bool, error) {
	if id < 0 {
		return false, ErrLegacyReadOnly
	}
	res, err := r.db.SQL().Exec(`UPDATE clipboard_entry SET favorite = ? WHERE id = ?`, boolInt(favorite), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Touch bumps use time and count on writable rows (no-op on read-only legacy rows).
func (r *Repo) Touch(id int64) error {
	if id < 0 {
		return nil
	}
	_, err := r.db.SQL().Exec(`UPDATE clipboard_entry SET last_used_utc = ?, use_count = use_count + 1 WHERE id = ?`,
		time.Now().Unix(), id)
	return err
}

// Delete removes one writable record and returns its referenced image path.
func (r *Repo) Delete(id int64) (PruneResult, error) {
	if id < 0 {
		return PruneResult{}, ErrLegacyReadOnly
	}
	tx, err := r.db.SQL().Begin()
	if err != nil {
		return PruneResult{}, err
	}
	defer tx.Rollback()
	var image sql.NullString
	_ = tx.QueryRow(`SELECT image_path FROM clipboard_entry WHERE id = ?`, id).Scan(&image)
	res, err := tx.Exec(`DELETE FROM clipboard_entry WHERE id = ?`, id)
	if err != nil {
		return PruneResult{}, err
	}
	n, _ := res.RowsAffected()
	var images []string
	if n > 0 && image.Valid && image.String != "" {
		images = append(images, image.String)
	}
	if err := tx.Commit(); err != nil {
		return PruneResult{}, err
	}
	return PruneResult{RemovedEntries: int(n), RemovedImages: images}, nil
}

// ClearAll removes every non-favorite row from writable clip.db (never touches legacyDB).
func (r *Repo) ClearAll() (PruneResult, error) {
	tx, err := r.db.SQL().Begin()
	if err != nil {
		return PruneResult{}, err
	}
	defer tx.Rollback()
	images, err := collectImages(tx, `favorite = 0`)
	if err != nil {
		return PruneResult{}, err
	}
	res, err := tx.Exec(`DELETE FROM clipboard_entry WHERE favorite = 0`)
	if err != nil {
		return PruneResult{}, err
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return PruneResult{}, err
	}
	return PruneResult{RemovedEntries: int(n), RemovedImages: images}, nil
}

// Trim applies retention limits on writable clip.db (never touches legacyDB).
func (r *Repo) Trim(maxEntries, maxAgeDays int) (PruneResult, error) {
	tx, err := r.db.SQL().Begin()
	if err != nil {
		return PruneResult{}, err
	}
	defer tx.Rollback()
	removed := 0
	var images []string

	if maxAgeDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -maxAgeDays).Unix()
		imgs, err := collectImages(tx, `favorite = 0 AND created_utc < ?`, cutoff)
		if err != nil {
			return PruneResult{}, err
		}
		images = append(images, imgs...)
		res, err := tx.Exec(`DELETE FROM clipboard_entry WHERE favorite = 0 AND created_utc < ?`, cutoff)
		if err != nil {
			return PruneResult{}, err
		}
		n, _ := res.RowsAffected()
		removed += int(n)
	}

	if maxEntries > 0 {
		var live int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM clipboard_entry WHERE favorite = 0`).Scan(&live); err != nil {
			return PruneResult{}, err
		}
		excess := live - maxEntries
		if excess > 0 {
			const oldest = `SELECT id FROM clipboard_entry WHERE favorite = 0 ORDER BY last_used_utc ASC LIMIT ?`
			imgs, err := collectImagesSubquery(tx, oldest, excess)
			if err != nil {
				return PruneResult{}, err
			}
			images = append(images, imgs...)
			res, err := tx.Exec(`DELETE FROM clipboard_entry WHERE id IN (`+oldest+`)`, excess)
			if err != nil {
				return PruneResult{}, err
			}
			n, _ := res.RowsAffected()
			removed += int(n)
		}
	}
	if err := tx.Commit(); err != nil {
		return PruneResult{}, err
	}
	return PruneResult{RemovedEntries: removed, RemovedImages: uniqueStrings(images)}, nil
}

// AllImagePaths lists every referenced image path in writable clip.db.
func (r *Repo) AllImagePaths() ([]string, error) {
	rows, err := r.db.SQL().Query(`SELECT image_path FROM clipboard_entry WHERE image_path IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Counts returns row counts by kind in writable clip.db.
func (r *Repo) Counts() (map[core.ClipboardKind]int, error) {
	rows, err := r.db.SQL().Query(`SELECT kind, COUNT(*) FROM clipboard_entry GROUP BY kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[core.ClipboardKind]int{}
	for rows.Next() {
		var kind, n int
		if err := rows.Scan(&kind, &n); err != nil {
			return nil, err
		}
		out[core.ClipboardKind(kind)] = n
	}
	return out, rows.Err()
}

func getTx(tx *sql.Tx, id int64) (Record, error) {
	row := tx.QueryRow(`SELECT id, kind, preview, body, hash, byte_size, width, height, image_path,
		payload_json, created_utc, last_used_utc, use_count, favorite, source_process, body_truncated
		FROM clipboard_entry WHERE id = ?`, id)
	return scanRecord(row)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRecord(s scanner) (Record, error) {
	var rec Record
	var kind int
	var body, imagePath, payload, source sql.NullString
	var favorite, truncated int
	err := s.Scan(&rec.ID, &kind, &rec.Preview, &body, &rec.Hash, &rec.ByteSize, &rec.Width, &rec.Height,
		&imagePath, &payload, &rec.CreatedUTC, &rec.LastUsedUTC, &rec.UseCount, &favorite,
		&source, &truncated)
	if err != nil {
		return rec, err
	}
	rec.Kind = core.ClipboardKind(kind)
	rec.Body = body.String
	rec.ImagePath = imagePath.String
	rec.PayloadJSON = payload.String
	rec.SourceProcess = source.String
	rec.Favorite = favorite != 0
	rec.BodyTruncated = truncated != 0
	return rec, nil
}

func collectImages(tx *sql.Tx, where string, args ...any) ([]string, error) {
	rows, err := tx.Query(`SELECT image_path FROM clipboard_entry WHERE image_path IS NOT NULL AND `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func collectImagesSubquery(tx *sql.Tx, sub string, args ...any) ([]string, error) {
	rows, err := tx.Query(`SELECT image_path FROM clipboard_entry WHERE image_path IS NOT NULL AND id IN (`+sub+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
