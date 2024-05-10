package global

import (
	"log"
	"os"
	"path/filepath"
)

var DataDir string
var DatabaseDir string
var ImageDir string

func Init() string {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	DatabaseDir = filepath.Join(dataDir, "db")
	ImageDir = filepath.Join(dataDir, "images")

	// create folders
	if err := os.MkdirAll(DatabaseDir, 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(ImageDir, 0755); err != nil {
		log.Fatal(err)
	}

	return dataDir
}
