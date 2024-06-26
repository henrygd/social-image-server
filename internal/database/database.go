package database

import (
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/henrygd/social-image-server/internal/global"
	_ "modernc.org/sqlite"
)

var db *sql.DB

type Image struct {
	Url      string
	File     string
	Date     string
	CacheKey string
}

func getCleanInterval() string {
	if cleanInterval, ok := os.LookupEnv("CACHE_TIME"); ok {
		return cleanInterval
	}
	return "30 days"
}

func Init() {
	slog.Debug("Initializing database", "CACHE_TIME", getCleanInterval())
	var err error
	db, err = sql.Open("sqlite", filepath.Join(global.DatabaseDir, "social-image-server.db"))
	if err != nil {
		log.Fatal(err)
	}
	// limit open connections to avoid SQLITE_BUSY
	db.SetMaxOpenConns(1)
	// create table
	if _, err = db.Exec(
		`CREATE TABLE IF NOT EXISTS images (
			url TEXT NOT NULL PRIMARY KEY,
			file TEXT NOT NULL,
			date DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	); err != nil {
		log.Fatal("Error creating table:", err)
	}
	// add index to url column
	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS url_index ON images (url);`); err != nil {
		log.Fatal("Error creating index:", err)
	}
	runDatabaseUpdates()
	Clean()
}

func AddImage(img *Image) error {
	slog.Debug("Adding image to database", "url", img.Url)
	// check if row with the same URL exists
	var file string
	row := db.QueryRow(
		`SELECT file FROM images WHERE url=?;`, img.Url,
	)
	err := row.Scan(&file)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	// If old row exists, update row and delete old file
	if file != "" {
		_, err = db.Exec("UPDATE images SET file = ?, cache_key = ? WHERE url = ?", img.File, img.CacheKey, img.Url)
		if err != nil {
			return err
		}
		if err = os.Remove(global.ImageDir + file); err != nil {
			return err
		}
		slog.Debug("Updated existing row", "url", img.Url)
		return nil
	}

	_, err = db.Exec("INSERT INTO images (url, file, cache_key) VALUES (?, ?, ?)", img.Url, img.File, img.CacheKey)
	if err != nil {
		return err
	}
	slog.Debug("New row inserted", "url", img.Url)
	return nil
}

func GetImage(url string) (*Image, error) {
	var image Image

	row := db.QueryRow(`SELECT * FROM images WHERE url=?`, url)

	err := row.Scan(&image.Url, &image.File, &image.Date, &image.CacheKey)
	if err != nil && err != sql.ErrNoRows {
		slog.Error(err.Error())
	}

	return &image, err
}

// Cleans up expired database data by deleting rows and their corresponding files.
//
// It retrieves the expiration time from the environment variable "CACHE_TIME" or defaults to "30 days".
//
// Returns an error if there was a problem querying the database or deleting the files.
func Clean() error {
	slog.Debug("Cleaning expired database data")
	cleanInterval := getCleanInterval()
	// grab rows so we can delete the files
	rows, err := db.Query(fmt.Sprintf(`SELECT file FROM images WHERE date <= DATETIME('now', '-%s');`, cleanInterval))
	if err != nil {
		return err
	}
	defer rows.Close()
	files := []string{}
	// loop rows to delete files
	for rows.Next() {
		var file string
		if err := rows.Scan(&file); err != nil {
			return err
		}
		files = append(files, file)
	}
	// delete rows
	_, err = db.Exec(
		fmt.Sprintf(`DELETE FROM images WHERE date <= DATETIME('now', '-%s');`, cleanInterval),
	)
	if err != nil {
		return err
	}
	// delete files
	for _, file := range files {
		if err = os.Remove(filepath.Join(global.ImageDir, file)); err != nil {
			return err
		}
	}
	slog.Debug("Cleaned expired rows / images", "count", len(files))
	return nil
}

// needed to add cache_key col between 0.0.3 and 0.0.4 releases
//
// move to init function on major release
func runDatabaseUpdates() {
	_, err := db.Exec(`ALTER TABLE images ADD COLUMN cache_key TEXT NOT NULL DEFAULT '';`)
	if err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			log.Fatal("Error adding cache_key column:", err)
		}
	}
}
