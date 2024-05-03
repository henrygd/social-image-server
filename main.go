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
	"github.com/henrygd/social-image-server/database"
)

func main() {
	// create folders
	err := os.MkdirAll("./data/images", 0755)
	if err != nil {
		log.Fatal(err)
	}
	err = os.MkdirAll("./data/db", 0755)
	if err != nil {
		log.Fatal(err)
	}

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
		// get supplied_url parameter
		supplied_url := r.URL.Query().Get("url")
		supplied_width := r.URL.Query().Get("width")
		supplied_delay := r.URL.Query().Get("delay")

		fmt.Println("url:", supplied_url)
		fmt.Println("width:", supplied_width)

		// validate url
		validatedUrl, err := validateUrl(supplied_url)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		fmt.Println("validated url:", validatedUrl)

		// set viewport dimensions
		var viewportWidth int64
		var viewportHeight int64
		if supplied_width != "" {
			viewportWidth, _ = strconv.ParseInt(supplied_width, 10, 64)
		}
		if viewportWidth == 0 || viewportWidth > 2400 {
			viewportWidth = 1400
		}
		viewportHeight = viewportWidth * 9 / 16

		// set delay
		var delay int64
		if supplied_delay != "" {
			delay, _ = strconv.ParseInt(supplied_delay, 10, 64)
			if delay > 10000 {
				delay = 10000
			}
		}

		// check database for image
		key := fmt.Sprintf("%s-%s-%d", validatedUrl, supplied_width, delay)

		img, err := database.GetImage(key)
		if err == nil {
			fmt.Printf("found image in database: %s - %s", img.File, img.Date)
			serveImage(w, r, img.File)
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
			chromedp.EmulateViewport(viewportWidth, viewportHeight),
			chromedp.Navigate(validatedUrl),
			chromedp.Sleep(time.Duration(delay) * time.Millisecond),
			chromedp.CaptureScreenshot(&buf),
		}); err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if err := os.WriteFile("./data/images/fullScreenshot.png", buf, 0o644); err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// add image to database
		if _, err := database.AddImage(&database.SocialImage{
			Key:  key,
			File: "./data/images/fullScreenshot.png",
		}); err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		serveImage(w, r, "./data/images/fullScreenshot.png")
	})

	// start server
	log.Println("Starting server on port 8080")

	http.ListenAndServe(":8080", router)
}

func serveImage(w http.ResponseWriter, r *http.Request, filename string) {
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
	domains := os.Getenv("SOCIAL_IMAGE_DOMAINS")
	if domains != "" && !strings.Contains(domains, u.Host) {
		return "", errors.New("domain not allowed")
	}

	// strip query from url
	supplied_url = strings.Split(supplied_url, "?")[0]

	return supplied_url, nil
}
