package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/henrygd/social-image-server/internal/browsercontext"
	"github.com/henrygd/social-image-server/internal/concurrency"
	"github.com/henrygd/social-image-server/internal/database"
	"github.com/henrygd/social-image-server/internal/global"
	"github.com/henrygd/social-image-server/internal/scraper"
	"github.com/henrygd/social-image-server/internal/update"
)

var version = "0.0.6"

var allowedDomainsMap map[string]bool

var imageOptions = struct {
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

func main() {
	// handle flags
	flagVersion := flag.Bool("v", false, "Print version")
	flagUpdate := flag.Bool("update", false, "Update to latest version")
	flag.Parse()

	if *flagVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	if *flagUpdate {
		update.Run(version)
		os.Exit(0)
	}

	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		switch logLevel {
		case "debug":
			slog.SetLogLoggerLevel(slog.LevelDebug)
		case "warn":
			slog.SetLogLoggerLevel(slog.LevelWarn)
		case "error":
			slog.SetLogLoggerLevel(slog.LevelError)
		}
	}

	slog.Info("Social Image Server", "v", version)

	router := setUpRouter()

	// start cleanup routine
	go cleanup()

	// start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	slog.Info("Starting server", "port", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatal(err)
	}
}

func setUpRouter() *http.ServeMux {
	global.Init()
	database.Init()
	browsercontext.Init()

	// create map of allowed allowedDomains for quick lookup
	if allowedDomains, ok := os.LookupEnv("ALLOWED_DOMAINS"); ok {
		slog.Debug("ALLOWED_DOMAINS", "value", allowedDomains)
		domains := strings.Split(allowedDomains, ",")
		allowedDomainsMap = make(map[string]bool, len(domains))
		for _, domain := range domains {
			domain = strings.TrimSpace(domain)
			if domain != "" {
				allowedDomainsMap[domain] = true
			}
		}
	}

	// set image format
	if os.Getenv("IMG_FORMAT") == "png" {
		imageOptions.Format = "png"
		imageOptions.Extension = ".png"
	}
	// set image width
	if width, ok := os.LookupEnv("IMG_WIDTH"); ok {
		var err error
		imageOptions.Width, err = strconv.ParseFloat(width, 64)
		if err != nil || imageOptions.Width < 1000 || imageOptions.Width > 2500 {
			slog.Error("Invalid IMG_WIDTH", "value", width, "min", 1000, "max", 2500)
			os.Exit(1)
		}
	}
	// set image quality
	if quality, ok := os.LookupEnv("IMG_QUALITY"); ok {
		var err error
		imageOptions.Quality, err = strconv.ParseInt(quality, 10, 64)
		if err != nil || imageOptions.Quality < 1 || imageOptions.Quality > 100 {
			slog.Error("Invalid IMG_QUALITY", "value", quality, "min", 1, "max", 100)
			os.Exit(1)
		}
	}

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
			slog.Debug("Regen key validated", "url", validatedUrl)
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
				slog.Debug("Found cached image", "url", validatedUrl, "cache_key", paramCacheKey)
				serveImage(w, r, global.ImageDir+cachedImage.File, "HIT", "2")
				return
			}
		}

		// should get here only if
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
			slog.Debug("Cache key does not match", "url", validatedUrl, "request", paramCacheKey, "origin", pageCacheKey)
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

	return router
}

func takeScreenshot(validatedUrl string, urlKey string, pageCacheKey string, params url.Values) (filepath string, err error) {
	slog.Debug("Taking screenshot", "url", validatedUrl)
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
	scale := imageOptions.Width / float64(viewportWidth)

	// set delay
	paramDelay := params.Get("delay")
	var delay int64
	if paramDelay != "" {
		delay, _ = strconv.ParseInt(paramDelay, 10, 64)
		if delay > 10000 {
			delay = 10000
		}
	}

	var taskCtx context.Context
	var cancel context.CancelFunc
	if browsercontext.IsRemoteBrowser {
		// if using remote browser, use remote context
		taskCtx, cancel = browsercontext.GetRemoteContext()
	} else {
		var resetBrowserTimer func()
		taskCtx, cancel, resetBrowserTimer = browsercontext.GetTaskContext()
		defer resetBrowserTimer()
	}
	defer cancel()

	// create file for screenshot
	f, err := os.CreateTemp(global.ImageDir, "*"+imageOptions.Extension)
	if err != nil {
		return "", err
	}
	defer f.Close()
	filepath = f.Name()

	tasks := chromedp.Tasks{}

	// set prefers dark mode
	if params.Get("dark") == "true" {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			emulatedMedia := emulation.SetEmulatedMedia()
			emulatedMedia.Features = append(emulatedMedia.Features, &emulation.MediaFeature{Name: "prefers-color-scheme", Value: "dark"})
			return emulatedMedia.Do(ctx)
		}))
	}

	// navigate to url
	tasks = append(tasks,
		// chromedp.Emulate(device.IPad),
		chromedp.EmulateViewport(viewportWidth, viewportHeight, chromedp.EmulateScale(scale)),
		chromedp.Navigate(validatedUrl),
		// chromedp.Evaluate(`document.documentElement.style.overflow = 'hidden'`, nil),
	)
	// add delay
	if delay != 0 {
		tasks = append(tasks, chromedp.Sleep(time.Duration(delay)*time.Millisecond))
	}
	// take screenshot
	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		format := page.CaptureScreenshotFormat(imageOptions.Format)
		buf, err := page.CaptureScreenshot().WithFormat(format).WithQuality(imageOptions.Quality).Do(ctx)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath, buf, 0644)
	}))

	if err = chromedp.Run(taskCtx, tasks); err != nil {
		// clean up empty file if tasks failed
		os.Remove(filepath)
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
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	//lint:ignore S1000 This loop is intentionally infinite and waits on ticker
	for {
		select {
		case <-ticker.C:
			if err := database.Clean(); err != nil {
				slog.Error("Error cleaning database", "error", err)
			}
			concurrency.CleanUrlMutexes(time.Now())
		}
	}
}

func serveImage(w http.ResponseWriter, r *http.Request, filename, status, code string) {
	w.Header().Set("X-Og-Cache", status)
	w.Header().Set("X-Og-Code", code)
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
	if allowedDomainsMap != nil && !allowedDomainsMap[u.Host] {
		return "", errors.New("domain " + u.Host + " not allowed")
	}

	return u.Scheme + "://" + u.Host + u.Path, nil
}

// Check if the status code of a url is 200 OK and extract cache_key
// possible to do in browser but this is more efficient for 404s / bad cache keys
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
	slog.Error("Error serving image", "error", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}
