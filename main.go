package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/henrygd/social-image-server/database"
)

var imgDir = "./data/images"

var lastClean time.Time

func main() {
	// create folders
	err := os.MkdirAll(imgDir, 0755)
	if err != nil {
		log.Fatal(err)
	}
	err = os.MkdirAll(database.DatabaseDir, 0755)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Setting up libvips")
	vips.LoggingSettings(nil, vips.LogLevelWarning)
	vips.Startup(nil)
	defer vips.Shutdown()

	// create database
	err = database.Init()
	if err != nil {
		log.Fatal(err)
		return
	}

	router := http.NewServeMux()

	// redirect to github page if index
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://github.com/henrygd/social-image-server", http.StatusFound)
	})

	router.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		// clean old images every hour
		now := time.Now()
		if now.Sub(lastClean) > time.Hour {
			err = database.Clean(imgDir)
			if err != nil {
				log.Println(err)
			}
			lastClean = now
		}

		// get supplied_url parameter
		params := r.URL.Query()

		// validate url
		validatedUrl, err := validateUrl(params.Get("url"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// fmt.Println("validated url:", validatedUrl)

		// set viewport dimensions
		supplied_width := params.Get("width")
		var viewportWidth int64
		var viewportHeight int64
		if supplied_width != "" {
			viewportWidth, _ = strconv.ParseInt(supplied_width, 10, 64)
		}
		if viewportWidth == 0 || viewportWidth > 2400 {
			viewportWidth = 1400
		}
		viewportHeight = viewportWidth * 9 / 16

		// calculate scale to make image 2200px wide
		scale := 2200 / float64(viewportWidth)

		// set delay
		supplied_delay := params.Get("delay")
		var delay int64
		if supplied_delay != "" {
			delay, _ = strconv.ParseInt(supplied_delay, 10, 64)
			if delay > 10000 {
				delay = 10000
			}
		}

		// check database for image
		key := fmt.Sprintf("%s-%d-%d", strings.TrimSuffix(validatedUrl, "/"), viewportWidth, delay)

		img, err := database.GetImage(key)
		if err == nil {
			serveImage(w, r, imgDir+img.File)
			return
		}

		// check that url provides 200 response before generating image
		ok := checkUrlOk(validatedUrl)
		if !ok {
			http.Error(w, "Requested URL not found", http.StatusNotFound)
			return
		}

		// create context
		ctx, cancel := chromedp.NewContext(
			context.Background(),
			// chromedp.WithDebugf(log.Printf),
		)
		defer cancel()

		var buf []byte

		// capture viewport, returning png
		if err := chromedp.Run(ctx, chromedp.Tasks{
			chromedp.EmulateViewport(viewportWidth, viewportHeight, chromedp.EmulateScale(scale)),
			chromedp.Navigate(validatedUrl),
			chromedp.Sleep(time.Duration(delay) * time.Millisecond),
			chromedp.CaptureScreenshot(&buf),
		}); err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// save image
		f, err := os.CreateTemp(imgDir, "*.jpg")
		if err != nil {
			log.Fatal(err)
		}
		filepath := f.Name()

		optimized, err := vips.NewImageFromBuffer(buf)
		if err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		imagebytes, _, err := optimized.ExportJpeg(&vips.JpegExportParams{
			Quality:   90,
			Interlace: true,
		})
		if err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(filepath, imagebytes, 0o644); err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// add image to database
		if _, err := database.AddImage(&database.SocialImage{
			Key:  key,
			File: strings.TrimPrefix(filepath, imgDir),
		}); err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		serveImage(w, r, filepath)
	})

	// start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Starting server on port", port)

	http.ListenAndServe(":"+port, router)
}

func serveImage(w http.ResponseWriter, r *http.Request, filename string) {
	// w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, filename)
}

func validateUrl(supplied_url string) (string, error) {
	if supplied_url == "" {
		return "", errors.New("no url supplied")
	}

	u, err := url.Parse(supplied_url)

	valid := err == nil && strings.HasPrefix(u.Scheme, "http") && u.Host != ""
	if !valid {
		return "", errors.New("invalid url")
	}

	// check if host is in whitelist
	domains := os.Getenv("ALLOWED_DOMAINS")
	if domains != "" && !strings.Contains(domains, u.Host) {
		return "", errors.New("domain not allowed")
	}

	// strip query from url
	supplied_url = strings.Split(supplied_url, "?")[0]

	return supplied_url, nil
}

// Check if the status code is 200 OK
func checkUrlOk(validatedUrl string) bool {
	resp, err := http.Get(validatedUrl)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
