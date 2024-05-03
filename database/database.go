package database

import (
	"context"
	"database/sql"
	"log"
	"os"

	_ "modernc.org/sqlite"
)

var db *sql.DB

var DatabaseDir = "./data/db"

type SocialImage struct {
	Key  string
	File string
	Date string
}

func Init() error {
	log.Println("Initializing database")

	// make sure directory exists
	err := os.MkdirAll(DatabaseDir, 0755)
	if err != nil {
		return err
	}

	db, err = sql.Open("sqlite", DatabaseDir+"/social-images.db")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(
		context.Background(),
		`CREATE TABLE IF NOT EXISTS images (
			key TEXT NOT NULL PRIMARY KEY,
			file TEXT NOT NULL,
			date DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	)
	if err != nil {
		return err
	}
	return nil
}

func AddImage(a *SocialImage) (int64, error) {
	result, err := db.ExecContext(
		context.Background(),
		`INSERT INTO images (key, file) VALUES (?,?);`, a.Key, a.File,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetImage(key string) (SocialImage, error) {
	var socialImage SocialImage

	row := db.QueryRowContext(
		context.Background(),
		`SELECT * FROM images WHERE key=?`, key,
	)

	err := row.Scan(&socialImage.Key, &socialImage.File, &socialImage.Date)

	return socialImage, err
}
