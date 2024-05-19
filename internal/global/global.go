package global

import (
	"log"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

var DatabaseDir string
var ImageDir string
var TemplateDir string
var RegenKey string
var AllowedDomainsMap map[string]bool

var ImageOptions = struct {
	Format    string
	Extension string
	Quality   int64
	Width     float64
}{
	Format:    "jpeg",
	Extension: ".jpg",
	Quality:   92,
	Width:     2000,
}

type ReqData struct {
	ValidatedURL string
	UrlKey       string
	CacheKey     string
	Params       url.Values
	Template     string
}

func Init() (dataDir string) {
	dataDir = os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	slog.Debug("DATA_DIR", "value", dataDir)
	DatabaseDir = filepath.Join(dataDir, "db")
	ImageDir = filepath.Join(dataDir, "images")
	TemplateDir = filepath.Join(dataDir, "templates")

	// create folders
	if err := os.MkdirAll(DatabaseDir, 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(ImageDir, 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(TemplateDir, 0755); err != nil {
		log.Fatal(err)
	}
	// set image format
	if os.Getenv("IMG_FORMAT") == "png" {
		ImageOptions.Format = "png"
		ImageOptions.Extension = ".png"
	}
	// set image width
	if width, ok := os.LookupEnv("IMG_WIDTH"); ok {
		var err error
		ImageOptions.Width, err = strconv.ParseFloat(width, 64)
		if err != nil || ImageOptions.Width < 1000 || ImageOptions.Width > 2500 {
			slog.Error("Invalid IMG_WIDTH", "value", width, "min", 1000, "max", 2500)
			os.Exit(1)
		}
	}
	// set image quality
	if quality, ok := os.LookupEnv("IMG_QUALITY"); ok {
		var err error
		ImageOptions.Quality, err = strconv.ParseInt(quality, 10, 64)
		if err != nil || ImageOptions.Quality < 1 || ImageOptions.Quality > 100 {
			slog.Error("Invalid IMG_QUALITY", "value", quality, "min", 1, "max", 100)
			os.Exit(1)
		}
	}
	// set regen key
	RegenKey = os.Getenv("REGEN_KEY")

	return dataDir
}
