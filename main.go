package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/henrygd/social-image-server/internal/browsercontext"
	"github.com/henrygd/social-image-server/internal/concurrency"
	"github.com/henrygd/social-image-server/internal/database"
	"github.com/henrygd/social-image-server/internal/global"
	"github.com/henrygd/social-image-server/internal/scraper"
	"github.com/henrygd/social-image-server/internal/screenshot"
	"github.com/henrygd/social-image-server/internal/templates"
	"github.com/henrygd/social-image-server/internal/update"
)

var version = "0.0.6"

var allowedDomainsMap map[string]bool

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

	router := http.NewServeMux()

	// endpoints
	router.HandleFunc("/capture", handleCaptureRoute)
	router.HandleFunc("/template/{templateName}", handleTemplateRoute)
	router.HandleFunc("/template/{templateName}/", handleTemplateRoute)
	// get is previous name for capture route - leaving for compatibility
	router.HandleFunc("/get", handleCaptureRoute)

	// help redirects to github readme
	router.HandleFunc("/help", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://github.com/henrygd/social-image-server/blob/main/readme.md", http.StatusFound)
	})

	return router
}

func handleTemplateRoute(w http.ResponseWriter, r *http.Request) {
	templateName := r.PathValue("templateName")
	// check that template directory exists
	if !templates.IsValid(templateName) {
		http.Error(w, "Invalid template", http.StatusBadRequest)
		return
	}
	// get url query params
	params := r.URL.Query()

	validatedURL, err := validateUrl(params.Get("url"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// key for url in database / mutexes
	urlKey := strings.TrimSuffix(validatedURL, "/")
	// lock the mutex associated with the url
	mutex := concurrency.GetOrCreateUrlMutex(urlKey)
	mutex.Lock()
	defer mutex.Unlock()

	requestCacheKey := makeCacheKey(r.URL)

	// if _regen_ param is valid, regenerate screenshot and return
	if isRegenRequest(&params) {
		slog.Debug("Regen key validated", "template", templateName)
		if filepath, err := screenshot.Template(templateName, urlKey, requestCacheKey, &params); err == nil {
			serveImage(w, r, filepath, "MISS", "1")
		} else {
			handleServerError(w, err)
		}
		return
	}

	// check database for image
	// var cachedImage database.TemplateImage
	cachedImage, _ := database.GetImage(urlKey)

	// has cached image and request url matches cache key for url - return cached image
	if cachedImage.File != "" && cachedImage.CacheKey == requestCacheKey {
		slog.Debug("Found cached image", "url", validatedURL, "cache_key", cachedImage.CacheKey)
		serveImage(w, r, filepath.Join(global.ImageDir, cachedImage.File), "HIT", "2")
		return
	}

	// check origin url before using browser
	ok, originOgURL := checkUrlOk(validatedURL)
	if !ok {
		http.Error(w, "Could not connect to origin URL", http.StatusBadGateway)
		return
	}

	// has cached image but origin og url does not match request - return cached image
	originCacheKey := makeCacheKey(originOgURL)
	if cachedImage.File != "" && requestCacheKey != originCacheKey {
		slog.Debug("Request image does not match origin", "req", requestCacheKey, "origin", originCacheKey)
		serveImage(w, r, global.ImageDir+cachedImage.File, "HIT", "3")
		return
	}

	// generate image.
	// should only get here if:
	// 1. url is not cached at all
	// 2. origin og url matches request (origin updated, our db is stale)
	if filepath, err := screenshot.Template(validatedURL, urlKey, requestCacheKey, &params); err == nil {
		serveImage(w, r, filepath, "MISS", "0")
	} else {
		handleServerError(w, err)
	}
}

func handleCaptureRoute(w http.ResponseWriter, r *http.Request) {
	// get supplied_url parameter
	params := r.URL.Query()

	// validate url
	validatedURL, err := validateUrl(params.Get("url"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// key for url in database / mutexes
	urlKey := strings.TrimSuffix(validatedURL, "/")

	// lock the mutex associated with the url
	// todo maybe change the url mutex map to store other useful things
	// like status to avoid re-requests, or html for one min to avoid DoS
	mutex := concurrency.GetOrCreateUrlMutex(urlKey)
	mutex.Lock()
	defer mutex.Unlock()

	requestCacheKey := makeCacheKey(r.URL)

	// if _regen_ param is valid, generate screenshot and return
	if isRegenRequest(&params) {
		slog.Debug("Regen key validated", "url", validatedURL)
		if ok, _ := checkUrlOk(validatedURL); !ok {
			http.Error(w, "Could not connect to origin URL", http.StatusBadGateway)
			return
		}
		// take screenshot
		if filepath, err := screenshot.Capture(validatedURL, urlKey, requestCacheKey, &params); err == nil {
			serveImage(w, r, filepath, "MISS", "1")
		} else {
			handleServerError(w, err)
		}
		return
	}

	// check database for image
	cachedImage, _ := database.GetImage(urlKey)

	// has cached image and request url matches cache key for url - return cached image
	if cachedImage.File != "" && cachedImage.CacheKey == requestCacheKey {
		slog.Debug("Found cached image", "url", validatedURL, "cache_key", cachedImage.CacheKey)
		serveImage(w, r, filepath.Join(global.ImageDir, cachedImage.File), "HIT", "2")
		return
	}

	// check origin url before using browser
	ok, originOgURL := checkUrlOk(validatedURL)
	if !ok {
		http.Error(w, "Could not connect to origin URL", http.StatusBadGateway)
		return
	}

	// has cached image but origin og url does not match request - return cached image
	originCacheKey := makeCacheKey(originOgURL)
	if cachedImage.File != "" && requestCacheKey != originCacheKey {
		slog.Debug("Request image does not match origin", "req", requestCacheKey, "origin", originCacheKey)
		serveImage(w, r, global.ImageDir+cachedImage.File, "HIT", "3")
		return
	}

	// generate image.
	// should only get here if:
	// 1. url is not cached at all
	// 2. origin og url matches request (origin updated, our db is stale)
	if filepath, err := screenshot.Capture(validatedURL, urlKey, requestCacheKey, &params); err == nil {
		serveImage(w, r, filepath, "MISS", "0")
	} else {
		handleServerError(w, err)
	}
}

// cleans up old images and url mutexes, sleeps for an hour between cleaning cycles
func cleanup() {
	// drift one second to avoid "1 hour" cache time conflict
	ticker := time.NewTicker(time.Hour + time.Second)
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

// checks url.Values to verify request is a valid regeneration request
func isRegenRequest(params *url.Values) bool {
	v := params.Get("_regen_")
	return v != "" && (v == os.Getenv("REGEN_KEY"))
}

// validates a supplied URL and returns a formatted URL string.
func validateUrl(suppliedUrl string) (string, error) {
	if suppliedUrl == "" {
		return "", errors.New("no url supplied")
	}

	if !strings.HasPrefix(suppliedUrl, "https://") && !strings.HasPrefix(suppliedUrl, "http://") {
		suppliedUrl = "https://" + suppliedUrl
	}

	u, err := url.Parse(suppliedUrl)

	if err != nil {
		return "", errors.New("invalid url")
	}

	// check if host is in whitelist
	if allowedDomainsMap != nil && !allowedDomainsMap[u.Host] {
		return "", errors.New("domain " + u.Host + " not allowed")
	}

	return u.Scheme + "://" + u.Host + u.Path, nil
}

// Check if the status code of a url is 200 OK and extract the page's og image url
// possible to do in browser but this is more efficient for 404s / bad cache keys
func checkUrlOk(validatedUrl string) (ok bool, ogImageUrl string) {
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
	return true, scraper.FindOgUrl(doc)
}

// Generates a cache key based on the input URL path and query parameters.
//
// It takes a string representing a URL or *url.URL and returns a string.
func makeCacheKey(input interface{}) string {
	var u *url.URL
	switch v := input.(type) {
	case *url.URL:
		u = v
	case string:
		u, _ = url.Parse(v)
	default:
		return ""
	}
	if !strings.HasPrefix(u.Path, "/template") {
		// we don't need path for capture route and this maintains
		// consistency with images cached using /get
		u.Path = ""
	}
	params := u.Query()
	params.Del("_regen_")
	return u.Path + params.Encode()
}

func handleServerError(w http.ResponseWriter, err error) {
	slog.Error("Error serving image", "error", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}
