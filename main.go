package main

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"image/png"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/henrygd/social-image-server/database"
)

var imgDir = "./data/images"
var lastClean time.Time
var remoteUrl = os.Getenv("REMOTE_URL")
var allowedDomains = os.Getenv("ALLOWED_DOMAINS")
var allowedDomainsMap = make(map[string]bool)

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

	// create database
	err = database.Init()
	if err != nil {
		log.Fatal(err)
		return
	}

	// create map of allowed allowedDomains for quick lookup
	for _, domain := range strings.Split(allowedDomains, ",") {
		domain = strings.TrimSpace(domain)
		if domain != "" {
			allowedDomainsMap[domain] = true
		}
	}

	// create allocator context for use with creating a browser context later
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("--font-render-hinting", "none"),
	)
	var cancel context.CancelFunc
	globalContext, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	var remoteContext context.Context
	if remoteUrl != "" {
		remoteContext, cancel = chromedp.NewRemoteAllocator(globalContext, remoteUrl)
	}
	defer cancel()

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
		dbUrl := strings.TrimSuffix(validatedUrl, "/")

		regen := params.Get("regen") != "" && (params.Get("regen") == os.Getenv("REGEN_KEY"))

		if regen {
			// if regen key is provided, delete image from database
			err = database.DeleteImage(imgDir, dbUrl)
			if err != nil {
				log.Println(err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
		} else {
			// else check database for cached image
			img, err := database.GetImage(dbUrl)
			if err == nil {
				serveImage(w, r, imgDir+img.File)
				return
			}
		}

		// check that url provides 200 response before generating image
		ok := checkUrlOk(validatedUrl)
		if !ok {
			http.Error(w, "Requested URL not found", http.StatusNotFound)
			return
		}

		// create task context
		var parentContext = remoteContext
		if parentContext == nil {
			parentContext = globalContext
		}
		taskCtx, cancel := chromedp.NewContext(parentContext)
		defer cancel()

		var buf []byte

		// capture viewport, returning png
		if err := chromedp.Run(taskCtx, chromedp.Tasks{
			chromedp.EmulateViewport(viewportWidth, viewportHeight, chromedp.EmulateScale(scale)),
			chromedp.Navigate(validatedUrl),
			chromedp.Evaluate(`document.documentElement.style.overflow = 'hidden'`, nil),
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

		// Decode the PNG image bytes
		img, err := png.Decode(bytes.NewReader(buf))
		// encode the image as a JPEG file
		buff := new(bytes.Buffer)
		if err = jpeg.Encode(buff, img, &jpeg.Options{Quality: 88}); err != nil {
			return
		}
		if err := os.WriteFile(filepath, buff.Bytes(), 0o644); err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// add image to database
		if _, err := database.AddImage(&database.SocialImage{
			Url:  dbUrl,
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

	if !strings.HasPrefix(supplied_url, "https://") && !strings.HasPrefix(supplied_url, "http://") {
		supplied_url = "https://" + supplied_url
	}

	u, err := url.Parse(supplied_url)

	if err != nil {
		return "", errors.New("invalid url")
	}

	// check if host is in whitelist
	if allowedDomains != "" && !allowedDomainsMap[u.Host] {
		return "", errors.New("domain " + u.Host + " not allowed")
	}

	return u.Scheme + "://" + u.Host + u.Path, nil
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
