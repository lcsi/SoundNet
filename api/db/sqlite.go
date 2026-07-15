package db

import (
	"database/sql"
	"encoding/json"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"music-player/models"
)

type DB struct {
	conn *sql.DB
}

func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

func (d *DB) migrate() error {
	_, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS players (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			settings TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	// Create alarms table
	_, err = d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS alarms (
			id TEXT PRIMARY KEY,
			channel TEXT NOT NULL,
			type TEXT NOT NULL,
			data TEXT NOT NULL,
			enabled INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// Check if settings column exists, add it if not (for existing databases)
	var hasSettings bool
	row := d.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('players') WHERE name='settings'")
	var count int
	if err := row.Scan(&count); err == nil {
		hasSettings = count > 0
	}
	if !hasSettings {
		_, err = d.conn.Exec("ALTER TABLE players ADD COLUMN settings TEXT NOT NULL DEFAULT '{}'")
		if err != nil {
			return err
		}
	}

	_, err = d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS channels (
			name TEXT PRIMARY KEY,
			display_name TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`)
	return err
}

func (d *DB) GetPlayer(id string) (*models.Player, error) {
	row := d.conn.QueryRow("SELECT id, name, note, settings, created_at, updated_at FROM players WHERE id = ?", id)
	p := &models.Player{}
	var settingsJSON string
	err := row.Scan(&p.ID, &p.Name, &p.Note, &settingsJSON, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if settingsJSON != "" {
		json.Unmarshal([]byte(settingsJSON), &p.Settings)
	}
	return p, nil
}

func (d *DB) CreatePlayer(id string) (*models.Player, error) {
	now := time.Now()
	p := &models.Player{
		ID:        id,
		Name:      id,
		Note:      "",
		Settings:  models.PlayerSettings{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	settingsJSON, _ := json.Marshal(p.Settings)
	_, err := d.conn.Exec(
		"INSERT INTO players (id, name, note, settings, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		p.ID, p.Name, p.Note, string(settingsJSON), p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (d *DB) UpsertPlayer(id string) error {
	now := time.Now()
	// Try to insert; if already exists, just update the timestamp
	_, err := d.conn.Exec(
		"INSERT INTO players (id, name, note, settings, created_at, updated_at) VALUES (?, ?, '', '{}', ?, ?) ON CONFLICT(id) DO UPDATE SET updated_at = ?",
		id, id, now, now, now,
	)
	return err
}

func (d *DB) UpdatePlayer(id, name, note string, settings models.PlayerSettings) error {
	settingsJSON, _ := json.Marshal(settings)
	_, err := d.conn.Exec(
		"UPDATE players SET name = ?, note = ?, settings = ?, updated_at = ? WHERE id = ?",
		name, note, string(settingsJSON), time.Now(), id,
	)
	return err
}

func (d *DB) ListPlayers() ([]*models.Player, error) {
	rows, err := d.conn.Query("SELECT id, name, note, settings, created_at, updated_at FROM players ORDER BY updated_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []*models.Player
	for rows.Next() {
		p := &models.Player{}
		var settingsJSON string
		if err := rows.Scan(&p.ID, &p.Name, &p.Note, &settingsJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if settingsJSON != "" {
			json.Unmarshal([]byte(settingsJSON), &p.Settings)
		}
		players = append(players, p)
	}
	return players, nil
}

// ─── Channel CRUD ─────────────────────────────────────────────

// ChannelRecord represents a persisted channel in SQLite.
type ChannelRecord struct {
	Name        string
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// EnsureChannel creates a channel record if it doesn't exist; otherwise updates the timestamp.
func (d *DB) EnsureChannel(name string) error {
	now := time.Now()
	_, err := d.conn.Exec(
		`INSERT INTO channels (name, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET updated_at = ?`,
		name, name, now, now, now,
	)
	return err
}

// GetChannelRecord retrieves a channel record by name.
func (d *DB) GetChannelRecord(name string) (*ChannelRecord, error) {
	row := d.conn.QueryRow("SELECT name, display_name, created_at, updated_at FROM channels WHERE name = ?", name)
	r := &ChannelRecord{}
	err := row.Scan(&r.Name, &r.DisplayName, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// UpdateChannelDisplayName updates the display_name of a channel.
func (d *DB) UpdateChannelDisplayName(name, displayName string) error {
	_, err := d.conn.Exec(
		"UPDATE channels SET display_name = ?, updated_at = ? WHERE name = ?",
		displayName, time.Now(), name,
	)
	return err
}

// DeleteChannel removes a channel record from the database.
func (d *DB) DeleteChannel(name string) error {
	_, err := d.conn.Exec("DELETE FROM channels WHERE name = ?", name)
	return err
}

// ListChannelRecords returns all channel records from the database.
func (d *DB) ListChannelRecords() ([]*ChannelRecord, error) {
	rows, err := d.conn.Query("SELECT name, display_name, created_at, updated_at FROM channels ORDER BY updated_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*ChannelRecord
	for rows.Next() {
		r := &ChannelRecord{}
		if err := rows.Scan(&r.Name, &r.DisplayName, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// DeletePlayer removes a player record from the database.
func (d *DB) DeletePlayer(id string) error {
	_, err := d.conn.Exec("DELETE FROM players WHERE id = ?", id)
	return err
}

// ─── Alarm CRUD ────────────────────────────────────────────

// CreateAlarm persists a new alarm definition.
func (d *DB) CreateAlarm(alarm *models.Alarm) error {
	dataJSON, err := json.Marshal(alarm)
	if err != nil {
		return err
	}
	enabled := 0
	if alarm.Enabled {
		enabled = 1
	}
	_, err = d.conn.Exec(
		`INSERT INTO alarms (id, channel, type, data, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		alarm.ID, alarm.Channel, alarm.Type, string(dataJSON), enabled,
	)
	return err
}

// UpdateAlarm updates an existing alarm definition.
func (d *DB) UpdateAlarm(alarm *models.Alarm) error {
	dataJSON, err := json.Marshal(alarm)
	if err != nil {
		return err
	}
	enabled := 0
	if alarm.Enabled {
		enabled = 1
	}
	_, err = d.conn.Exec(
		`UPDATE alarms SET data = ?, enabled = ?, type = ?, updated_at = datetime('now') WHERE id = ?`,
		string(dataJSON), enabled, alarm.Type, alarm.ID,
	)
	return err
}

// DeleteAlarm removes an alarm by ID.
func (d *DB) DeleteAlarm(id string) error {
	_, err := d.conn.Exec("DELETE FROM alarms WHERE id = ?", id)
	return err
}

// GetAlarm retrieves a single alarm by ID.
func (d *DB) GetAlarm(id string) (*models.Alarm, error) {
	row := d.conn.QueryRow("SELECT data FROM alarms WHERE id = ?", id)
	var dataJSON string
	err := row.Scan(&dataJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var alarm models.Alarm
	if err := json.Unmarshal([]byte(dataJSON), &alarm); err != nil {
		return nil, err
	}
	return &alarm, nil
}

// ListChannelAlarms returns all alarms for a given channel.
func (d *DB) ListChannelAlarms(channel string) ([]*models.Alarm, error) {
	rows, err := d.conn.Query("SELECT data FROM alarms WHERE channel = ? ORDER BY created_at DESC", channel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alarms []*models.Alarm
	for rows.Next() {
		var dataJSON string
		if err := rows.Scan(&dataJSON); err != nil {
			return nil, err
		}
		var alarm models.Alarm
		if err := json.Unmarshal([]byte(dataJSON), &alarm); err != nil {
			return nil, err
		}
		alarms = append(alarms, &alarm)
	}
	return alarms, rows.Err()
}

// ListAllEnabledAlarms returns all enabled alarms across all channels.
func (d *DB) ListAllEnabledAlarms() ([]*models.Alarm, error) {
	rows, err := d.conn.Query("SELECT data FROM alarms WHERE enabled = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alarms []*models.Alarm
	for rows.Next() {
		var dataJSON string
		if err := rows.Scan(&dataJSON); err != nil {
			return nil, err
		}
		var alarm models.Alarm
		if err := json.Unmarshal([]byte(dataJSON), &alarm); err != nil {
			return nil, err
		}
		alarms = append(alarms, &alarm)
	}
	return alarms, rows.Err()
}

func (d *DB) Close() error {
	return d.conn.Close()
}
