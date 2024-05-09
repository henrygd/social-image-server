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

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/henrygd/social-image-server/internal/concurrency"
	"github.com/henrygd/social-image-server/internal/database"
	"github.com/henrygd/social-image-server/internal/global"
	"github.com/henrygd/social-image-server/internal/scraper"
)

var remoteUrl = os.Getenv("REMOTE_URL")
var allowedDomains = os.Getenv("ALLOWED_DOMAINS")
var allowedDomainsMap = make(map[string]bool)

// allocator context for use with creating a browser context later
var globalBrowserContext context.Context

func main() {
	if global.DataDir == "" {
		global.DataDir = "./data"
	}
	global.ImageDir = global.DataDir + "/images"
	global.DatabaseDir = global.DataDir + "/db"
	// create folders
	if err := os.MkdirAll(global.ImageDir, 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(global.DatabaseDir, 0755); err != nil {
		log.Fatal(err)
	}

	// create database
	if err := database.Init(); err != nil {
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

	var cancel context.CancelFunc
	if remoteUrl != "" {
		globalBrowserContext, cancel = chromedp.NewRemoteAllocator(context.Background(), remoteUrl)
	} else {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("font-render-hinting", "none"),
			chromedp.Flag("disable-font-subpixel-positioning", true),
		)
		font := os.Getenv("FONT_FAMILY")
		if font != "" {
			opts = append(opts, chromedp.Flag("system-font-family", font))
		}
		// var blankOpts []func(*chromedp.ExecAllocator)
		globalBrowserContext, cancel = chromedp.NewExecAllocator(context.Background(), opts...)
	}
	defer cancel()

	router := http.NewServeMux()

	// redirect to github page if index
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://github.com/henrygd/social-image-server", http.StatusFound)
	})

	router.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		// get supplied_url parameter
		params := r.URL.Query()

		// validate url
		validatedUrl, err := validateUrl(params.Get("url"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// key for url in database / mutexes
		urlKey := strings.TrimSuffix(validatedUrl, "/")

		// lock the mutex associated with the url
		// todo maybe change the url mutex map to store other useful things
		// like status to avoid re-requests, or html for one min to avoid DoS
		mutex := concurrency.GetOrCreateUrlMutex(urlKey)
		mutex.Lock()
		defer mutex.Unlock()

		// if _regen_ param is set, regenerate screenshot and return
		if regen := params.Get("_regen_") != "" && (params.Get("_regen_") == os.Getenv("REGEN_KEY")); regen {
			ok, pageCacheKey := checkUrlOk(validatedUrl)
			if !ok {
				http.Error(w, "Requested URL not found", http.StatusNotFound)
				return
			}
			// take screenshot
			if filepath, err := takeScreenshot(validatedUrl, urlKey, pageCacheKey, params); err == nil {
				serveImage(w, r, filepath, "MISS", "1")
			} else {
				handleServerError(w, err)
			}
			return
		}

		paramCacheKey := params.Get("cache_key")
		cachedImage, _ := database.GetImage(urlKey)

		// has cached image
		if cachedImage.File != "" {
			// if no cache_key in request and found cached image, return cached image
			// if cache_key param matches db cache key, return cached image
			if paramCacheKey == "" || paramCacheKey == cachedImage.CacheKey {
				serveImage(w, r, global.ImageDir+cachedImage.File, "HIT", "2")
				return
			}
		}

		// should only get here if
		// 1. image is not cached
		// 2. cache_key param is provided and doesn't match db cache key

		// check url response and cache_key before using browser
		ok, pageCacheKey := checkUrlOk(validatedUrl)
		if !ok {
			http.Error(w, "Requested URL not found", http.StatusNotFound)
			return
		}

		// if request has cache_key but pageCacheKey doesn't match
		if paramCacheKey != "" && pageCacheKey != paramCacheKey {
			// return cached image if it exists
			if cachedImage.File != "" {
				serveImage(w, r, global.ImageDir+cachedImage.File, "HIT", "3")
				return
			}
			// if no cached image, return error
			http.Error(w, "request cache_key does not match origin cache_key", http.StatusBadRequest)
			return
		}

		// if request doesn't meet above conditions, take screenshot
		if filepath, err := takeScreenshot(validatedUrl, urlKey, pageCacheKey, params); err == nil {
			serveImage(w, r, filepath, "MISS", "0")
		} else {
			handleServerError(w, err)
		}
	})

	// start cleanup routine
	go cleanup()

	// start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Starting server on port", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatal(err)
	}
}

func takeScreenshot(validatedUrl string, urlKey string, pageCacheKey string, params url.Values) (filepath string, err error) {
	log.Println("Taking screenshot for", validatedUrl)
	// add og-image-request parameter to url
	validatedUrl += "?og-image-request=true"

	// set viewport dimensions
	paramWidth := params.Get("width")
	var viewportWidth int64
	var viewportHeight int64
	if paramWidth != "" {
		viewportWidth, _ = strconv.ParseInt(paramWidth, 10, 64)
	}
	if viewportWidth == 0 {
		viewportWidth = 1400
	} else if viewportWidth > 2400 {
		viewportWidth = 2400
	} else if viewportWidth < 400 {
		viewportWidth = 400
	}
	// force 1.9:1 aspect ratio
	viewportHeight = int64(float64(viewportWidth) / 1.9)
	// calculate scale to make image 2000px wide
	scale := 2000 / float64(viewportWidth)

	// set delay
	paramDelay := params.Get("delay")
	var delay int64
	if paramDelay != "" {
		delay, _ = strconv.ParseInt(paramDelay, 10, 64)
		if delay > 10000 {
			delay = 10000
		}
	}

	// create task context
	taskCtx, cancel := chromedp.NewContext(globalBrowserContext)
	defer cancel()
	tasks := chromedp.Tasks{}

	// set prefers dark mode
	if params.Get("dark") == "true" {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			emulatedMedia := emulation.SetEmulatedMedia()
			emulatedMedia.Features = append(emulatedMedia.Features, &emulation.MediaFeature{Name: "prefers-color-scheme", Value: "dark"})
			return emulatedMedia.Do(ctx)
		}))
	}

	// navigate to url and capture screenshot to buf
	var buf = make([]byte, 0, 200*1024)
	tasks = append(tasks,
		// chromedp.Emulate(device.IPad),
		chromedp.EmulateViewport(viewportWidth, viewportHeight, chromedp.EmulateScale(scale)),
		chromedp.Navigate(validatedUrl),
		// chromedp.Evaluate(`document.documentElement.style.overflow = 'hidden'`, nil),
		chromedp.Sleep(time.Duration(delay)*time.Millisecond),
		chromedp.CaptureScreenshot(&buf))

	if err = chromedp.Run(taskCtx, tasks); err != nil {
		return "", err
	}

	// save image
	f, err := os.CreateTemp(global.ImageDir, "*.jpg")
	if err != nil {
		return "", err
	}
	filepath = f.Name()

	// decode the png
	decodedPng, err := png.Decode(bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	// encode the jpeg
	buff := new(bytes.Buffer)
	err = jpeg.Encode(buff, decodedPng, &jpeg.Options{Quality: 90})
	if err != nil {
		return "", err
	}
	// write image to file
	err = os.WriteFile(filepath, buff.Bytes(), 0o644)
	if err != nil {
		return "", err
	}
	// add image to database
	err = database.AddImage(&database.SocialImage{
		Url:      urlKey,
		File:     strings.TrimPrefix(filepath, global.ImageDir),
		CacheKey: pageCacheKey,
	})
	if err != nil {
		return "", err
	}

	return filepath, nil
}

// cleans up old images and url mutexes, sleeps for an hour between cleaning cycles
func cleanup() {
	for {
		if err := database.Clean(); err != nil {
			log.Println(err)
		}
		time.Sleep(time.Hour)
		concurrency.CleanUrlMutexes(time.Now())
	}
}

func serveImage(w http.ResponseWriter, r *http.Request, filename, status, code string) {
	// w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Og-Cache", status)
	w.Header().Set("X-Og-Code", code)
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

// Check if the status code of a url is 200 OK
func checkUrlOk(validatedUrl string) (bool, string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	req, err := http.NewRequest("GET", validatedUrl, nil)
	if err != nil {
		return false, ""
	}
	// make the request
	resp, err := client.Do(req)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()
	// Check if the status code is 200 OK
	if ok := resp.StatusCode == http.StatusOK; !ok {
		return false, ""
	}
	// Parse the HTML response
	doc, err := scraper.Parse(resp.Body)
	if err != nil {
		return false, ""
	}
	// find og:image meta tag and extract the url
	ogImageURL := scraper.FindOgUrl(doc)
	if ogImageURL == "" {
		return true, ""
	}
	// extract the cache_key parameter from the og:image url
	cacheKey, _ := scraper.ExtractCacheKey(ogImageURL)
	return true, cacheKey
}

func handleServerError(w http.ResponseWriter, err error) {
	log.Println(err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}
