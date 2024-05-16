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
	router.HandleFunc("/template", handleTemplateRoute)
	// get is previous name for capture route - leaving for compatibility
	router.HandleFunc("/get", handleCaptureRoute)

	// redirect to github docs if not found
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://github.com/henrygd/social-image-server/blob/main/readme.md", http.StatusFound)
	})

	return router
}

func handleTemplateRoute(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	templateName := params.Get("t")
	if templateName == "" {
		http.Error(w, "Missing t parameter", http.StatusBadRequest)
		return
	}
	// check that template directory exists
	if _, err := os.Stat(filepath.Join(global.TemplateDir, templateName)); os.IsNotExist(err) {
		http.Error(w, "Template not found", http.StatusBadRequest)
		return
	}
	// start static server to serve template
	server, _, err := templates.TempServer(templateName)
	if err != nil {
		handleServerError(w, err)
		return
	}
	defer server.Close()
	defer slog.Debug("Template server stopped")
	// defer listener.Close()
	serverUrl := "http://" + server.Addr
	slog.Debug("Taking screenshot", "template", templateName)
	filename, err := screenshot.Template(serverUrl, params)
	if err != nil {
		handleServerError(w, err)
		return
	}
	defer os.Remove(filename)
	serveImage(w, r, filename, "ok", "200")
}

func handleCaptureRoute(w http.ResponseWriter, r *http.Request) {
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
		if filepath, err := screenshot.Url(validatedUrl, urlKey, pageCacheKey, params); err == nil {
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
	if filepath, err := screenshot.Url(validatedUrl, urlKey, pageCacheKey, params); err == nil {
		serveImage(w, r, filepath, "MISS", "0")
	} else {
		handleServerError(w, err)
	}
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
