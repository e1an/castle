package events

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type EventType string

const (
	EventMotion EventType = "motion"
	EventObject EventType = "object"
)

type Event struct {
	ID         int64
	CameraID   string
	Type       EventType
	Label      string  // object class, empty for motion-only
	Score      float64 // detection confidence
	ClipPath   string  // relative path to recorded clip
	OccurredAt time.Time
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	return s, s.migrate()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			camera_id   TEXT NOT NULL,
			type        TEXT NOT NULL,
			label       TEXT,
			score       REAL,
			clip_path   TEXT,
			occurred_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_events_camera ON events(camera_id);
		CREATE INDEX IF NOT EXISTS idx_events_time   ON events(occurred_at);
	`)
	return err
}

func (s *Store) Insert(e *Event) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO events (camera_id, type, label, score, clip_path, occurred_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.CameraID, e.Type, e.Label, e.Score, e.ClipPath, e.OccurredAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) List(cameraID string, limit int) ([]Event, error) {
	rows, err := s.db.Query(
		`SELECT id, camera_id, type, label, score, clip_path, occurred_at
		 FROM events
		 WHERE (? = '' OR camera_id = ?)
		 ORDER BY occurred_at DESC LIMIT ?`,
		cameraID, cameraID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.CameraID, &e.Type, &e.Label, &e.Score, &e.ClipPath, &e.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
