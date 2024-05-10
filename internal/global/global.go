package global

import (
	"log"
	"os"
)

var DataDir = setUpDataDirectories()
var DatabaseDir string
var ImageDir string

func setUpDataDirectories() string {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	DatabaseDir = dataDir + "/db"
	ImageDir = dataDir + "/images"

	// create folders
	if err := os.MkdirAll(DatabaseDir, 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(ImageDir, 0755); err != nil {
		log.Fatal(err)
	}

	return dataDir
}
