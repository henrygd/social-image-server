package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "modernc.org/sqlite"
)

var db *sql.DB
var cleanInterval = os.Getenv("CACHE_TIME")

type SocialImage struct {
	Url  string
	File string
	Date string
}

func Init(dataDir string) error {
	log.Println("Initializing database")

	// set default clean interval
	if cleanInterval == "" {
		cleanInterval = "30 days"
	}

	var err error
	db, err = sql.Open("sqlite", dataDir+"/db/social-image-server.db")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(
		context.Background(),
		`CREATE TABLE IF NOT EXISTS images (
			url TEXT NOT NULL PRIMARY KEY,
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
		`INSERT INTO images (url, file) VALUES (?,?);`, a.Url, a.File,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetImage(url string) (SocialImage, error) {
	var socialImage SocialImage

	row := db.QueryRowContext(
		context.Background(),
		`SELECT * FROM images WHERE url=?`, url,
	)

	err := row.Scan(&socialImage.Url, &socialImage.File, &socialImage.Date)

	return socialImage, err
}

func Clean(dataDir string) error {
	// log.Println("Cleaning expired images")
	rows, err := db.QueryContext(
		context.Background(),
		fmt.Sprintf(`SELECT * FROM images WHERE date < DATETIME('now', '-%s');`, cleanInterval),
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	// loop rows
	for rows.Next() {

		var image SocialImage
		err := rows.Scan(
			&image.Url, &image.File, &image.Date,
		)
		if err != nil {
			return err
		}
		// log.Println("Cleaning image", image.url)
		err = os.Remove(dataDir + "/images" + image.File)
		if err != nil {
			return err
		}
	}
	// delete rows
	res, err := db.ExecContext(
		context.Background(),
		fmt.Sprintf(`DELETE FROM images WHERE date < DATETIME('now', '-%s');`, cleanInterval),
	)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	log.Println("Cleaned", rowsAffected, "expired images")
	return nil
}

func DeleteImage(imgDir string, url string) error {
	// log.Println("Deleting image", url)

	var image SocialImage
	row := db.QueryRowContext(
		context.Background(),
		`SELECT * FROM images WHERE url=?;`, url,
	)
	err := row.Scan(&image.Url, &image.File, &image.Date)

	// if we error here, it means image doesn't exist
	// but that's fine, we'll just return nil
	if err != nil {
		return nil
	}
	err = os.Remove(imgDir + image.File)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(
		context.Background(),
		`DELETE FROM images WHERE url=?;`, url,
	)
	if err != nil {
		return err
	}
	return nil
}