// db_setup.go

package main

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type DBConfig struct {
	Driver   string
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

func SetupDB(config DBConfig) (*sql.DB, error) {
	var db *sql.DB
	var err error

	switch config.Driver {
	case "sqlite3":
		db, err = sql.Open("sqlite3", config.DBName)
	case "postgres":
		connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			config.Host, config.Port, config.User, config.Password, config.DBName)
		db, err = sql.Open("postgres", connStr)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", config.Driver)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	err = createTables(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %v", err)
	}

	return db, nil
}

func createTables(db *sql.DB) error {
	// Create the original packages table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS packages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			version TEXT
		)
	`)
	if err != nil {
		return err
	}

	// Create the FTS5 virtual table
	_, err = db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS packages_fts USING fts5(
			name,
			version,
			content='packages',
			content_rowid='id'
		)
	`)
	if err != nil {
		return err
	}

	// Create triggers to keep the FTS index up to date
	_, err = db.Exec(`
		CREATE TRIGGER IF NOT EXISTS packages_ai AFTER INSERT ON packages BEGIN
			INSERT INTO packages_fts(rowid, name, version) VALUES (new.id, new.name, new.version);
		END;
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		CREATE TRIGGER IF NOT EXISTS packages_ad AFTER DELETE ON packages BEGIN
			INSERT INTO packages_fts(packages_fts, rowid, name, version) VALUES('delete', old.id, old.name, old.version);
		END;
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		CREATE TRIGGER IF NOT EXISTS packages_au AFTER UPDATE ON packages BEGIN
			INSERT INTO packages_fts(packages_fts, rowid, name, version) VALUES('delete', old.id, old.name, old.version);
			INSERT INTO packages_fts(rowid, name, version) VALUES (new.id, new.name, new.version);
		END;
	`)
	if err != nil {
		return err
	}

	return nil
}