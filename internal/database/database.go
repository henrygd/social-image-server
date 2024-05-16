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
var cleanInterval = os.Getenv("CACHE_TIME")

type Image interface {
	GetUrl() string
	GetTable() string
}

type CaptureImage struct {
	Url      string
	File     string
	Date     string
	CacheKey string
}

type TemplateImage struct {
	Url    string
	File   string
	Date   string
	Params string
}

func (img *CaptureImage) GetUrl() string  { return img.Url }
func (img *TemplateImage) GetUrl() string { return img.Url }

func (img *CaptureImage) GetTable() string  { return "images" }
func (img *TemplateImage) GetTable() string { return "template_images" }

func Init() {
	// set default clean interval
	if cleanInterval == "" {
		cleanInterval = "30 days"
	}

	slog.Debug("Initializing database", "CACHE_TIME", cleanInterval)

	var err error
	db, err = sql.Open("sqlite", filepath.Join(global.DatabaseDir, "social-image-server.db"))
	if err != nil {
		log.Fatal(err)
	}
	// create tables
	if _, err = db.Exec(
		`CREATE TABLE IF NOT EXISTS images (
			url TEXT NOT NULL PRIMARY KEY,
			file TEXT NOT NULL,
			date DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	); err != nil {
		log.Fatal(err)
	}
	if _, err = db.Exec(
		`CREATE TABLE IF NOT EXISTS template_images (
			url TEXT NOT NULL PRIMARY KEY,
			file TEXT NOT NULL,
			date DATETIME DEFAULT CURRENT_TIMESTAMP,
			params TEXT NOT NULL
		)`,
	); err != nil {
		log.Fatal(err)
	}
	// add index to url columns
	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS url_index ON images (url);`); err != nil {
		log.Fatal("Error creating index:", err)
	}
	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS t_url_index ON template_images (url);`); err != nil {
		log.Fatal("Error creating index:", err)
	}
	runDatabaseUpdates()
	Clean()
}

func AddImage(img Image) error {
	slog.Debug("Adding image to database", "url", img.GetUrl())
	// check if row with the same URL exists
	var existingFile string
	row := db.QueryRow(
		fmt.Sprintf("SELECT file FROM %s WHERE url=?;", img.GetTable()), img.GetUrl(),
	)
	err := row.Scan(&existingFile)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	// If old row exists, update row and delete old file
	if existingFile != "" {
		switch v := img.(type) {
		case *CaptureImage:
			_, err = db.Exec("UPDATE images SET file = ?, cache_key = ? WHERE url = ?", v.File, v.CacheKey, v.Url)
		case *TemplateImage:
			_, err = db.Exec("UPDATE template_images SET file = ?, params = ? WHERE url = ?", v.File, v.Params, v.Url)
		}
		if err != nil {
			return err
		}
		if err = os.Remove(filepath.Join(global.ImageDir, existingFile)); err != nil {
			return err
		}
		slog.Debug("Updated existing row", "url", img.GetUrl())
		return nil
	}
	// If old row doesn't exist, insert new row
	switch v := img.(type) {
	case *CaptureImage:
		_, err = db.Exec("INSERT INTO images (url, file, cache_key) VALUES (?, ?, ?)", v.Url, v.File, v.CacheKey)
	case *TemplateImage:
		_, err = db.Exec("INSERT INTO template_images (url, file, params) VALUES (?, ?, ?)", v.Url, v.File, v.Params)
	}
	if err != nil {
		return err
	}
	slog.Debug("New row inserted", "url", img.GetUrl())
	return nil
}

// Retrieves an image from the database and populates the Image interface
func GetImage(dest Image, url string) (err error) {
	row := db.QueryRow(
		fmt.Sprintf("SELECT * FROM %s WHERE url=?;", dest.GetTable()), url,
	)
	switch v := dest.(type) {
	case *CaptureImage:
		err = row.Scan(&v.Url, &v.File, &v.Date, &v.CacheKey)
	case *TemplateImage:
		err = row.Scan(&v.Url, &v.File, &v.Date, &v.Params)
	}
	if err != nil && err != sql.ErrNoRows {
		slog.Error(err.Error())
	}
	return err
}

func Clean() error {
	slog.Debug("Cleaning expired database data")
	tables := []string{"images", "template_images"}
	expiredFiles := []string{}

	for _, table := range tables {
		// grab rows so we can delete the files
		rows, err := db.Query(fmt.Sprintf(`SELECT file FROM %s WHERE date < DATETIME('now', '-%s');`, table, cleanInterval))
		if err != nil {
			return err
		}
		defer rows.Close()
		// loop rows to delete files
		for rows.Next() {
			var file string
			if err := rows.Scan(&file); err != nil {
				return err
			}
			expiredFiles = append(expiredFiles, file)
		}
		// delete rows
		_, err = db.Exec(
			fmt.Sprintf(`DELETE FROM %s WHERE date < DATETIME('now', '-%s');`, table, cleanInterval),
		)
		if err != nil {
			return err
		}
	}
	// delete files
	for _, file := range expiredFiles {
		if err := os.Remove(filepath.Join(global.ImageDir, file)); err != nil {
			return err
		}
	}
	slog.Debug("Cleaned expired rows / images", "count", len(expiredFiles))
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
