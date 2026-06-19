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
	ID           int64
	CameraID     string
	Type         EventType
	Label        string  // object class, empty for motion-only
	Score        float64 // detection confidence
	ClipPath     string  // relative path to recorded clip
	SnapshotPath string  // relative path to full-frame JPEG
	CropPath     string  // relative path to face/subject crop JPEG
	OccurredAt   time.Time
}

type PushSubscription struct {
	Endpoint string
	P256DH   string
	Auth     string
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
	if _, err := s.db.Exec(`
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
		CREATE TABLE IF NOT EXISTS push_subscriptions (
			endpoint TEXT PRIMARY KEY,
			p256dh   TEXT NOT NULL,
			auth     TEXT NOT NULL
		);
	`); err != nil {
		return err
	}
	// Additive column migrations — probe with a dummy SELECT so we don't rely
	// on driver-specific error strings or pragma table-function availability.
	for _, col := range []string{"snapshot_path", "crop_path"} {
		if _, err := s.db.Exec(`SELECT ` + col + ` FROM events LIMIT 0`); err != nil {
			if _, err2 := s.db.Exec(`ALTER TABLE events ADD COLUMN ` + col + ` TEXT`); err2 != nil {
				return err2
			}
		}
	}
	return nil
}

func (s *Store) UpsertPushSubscription(endpoint, p256dh, auth string) error {
	_, err := s.db.Exec(`
		INSERT INTO push_subscriptions (endpoint, p256dh, auth) VALUES (?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET p256dh=excluded.p256dh, auth=excluded.auth`,
		endpoint, p256dh, auth)
	return err
}

func (s *Store) RemovePushSubscription(endpoint string) error {
	_, err := s.db.Exec(`DELETE FROM push_subscriptions WHERE endpoint = ?`, endpoint)
	return err
}

func (s *Store) ListPushSubscriptions() ([]PushSubscription, error) {
	rows, err := s.db.Query(`SELECT endpoint, p256dh, auth FROM push_subscriptions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PushSubscription
	for rows.Next() {
		var sub PushSubscription
		if err := rows.Scan(&sub.Endpoint, &sub.P256DH, &sub.Auth); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Store) Insert(e *Event) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO events (camera_id, type, label, score, clip_path, snapshot_path, crop_path, occurred_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.CameraID, e.Type, e.Label, e.Score, e.ClipPath, e.SnapshotPath, e.CropPath, e.OccurredAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) List(cameraID string, limit int) ([]Event, error) {
	rows, err := s.db.Query(
		`SELECT id, camera_id, type, label, score, clip_path, snapshot_path, crop_path, occurred_at
		 FROM events
		 WHERE (? = '' OR camera_id = ?)
		 ORDER BY occurred_at DESC LIMIT ?`,
		cameraID, cameraID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.CameraID, &e.Type, &e.Label, &e.Score, &e.ClipPath, &e.SnapshotPath, &e.CropPath, &e.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
