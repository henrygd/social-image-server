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
	"golang.org/x/net/html"
)

var version = "0.0.6"

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
	port, ok := os.LookupEnv("PORT")
	if !ok {
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
		global.AllowedDomainsMap = make(map[string]bool, len(domains))
		for _, domain := range domains {
			domain = strings.TrimSpace(domain)
			if domain != "" {
				global.AllowedDomainsMap[domain] = true
			}
		}
	}

	router := http.NewServeMux()

	// endpoints
	router.HandleFunc("/capture", handleImageRequest)
	router.HandleFunc("/template/{templateName}", handleImageRequest)
	router.HandleFunc("/template/{templateName}/", handleImageRequest)
	// get is previous name for capture route - leaving for compatibility
	router.HandleFunc("/get", handleImageRequest)

	// help redirects to github readme
	router.HandleFunc("/help", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://github.com/henrygd/social-image-server/blob/main/readme.md", http.StatusFound)
	})

	return router
}

func handleImageRequest(w http.ResponseWriter, r *http.Request) {
	var err error
	var reqData global.ReqData

	// if template, check that template directory exists
	if reqData.Template = r.PathValue("templateName"); reqData.Template != "" {
		if !templates.IsValid(reqData.Template) {
			http.Error(w, "Invalid template", http.StatusBadRequest)
			return
		}
	}

	// get url query params
	reqData.Params = r.URL.Query()

	reqData.ValidatedURL, err = validateUrl(reqData.Params.Get("url"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// key for url in database / mutexes
	reqData.UrlKey = strings.TrimSuffix(reqData.ValidatedURL, "/")
	// lock the mutex associated with the url
	mutex := concurrency.GetOrCreateUrlMutex(reqData.UrlKey)
	mutex.Lock()
	defer mutex.Unlock()

	reqData.CacheKey = makeCacheKey(r.URL)

	// if _regen_ param is valid, regenerate screenshot and return
	if isRegenRequest(&reqData.Params) {
		if reqData.Template != "" {
			slog.Debug("Regen key validated", "template", reqData.Template)
		} else {
			slog.Debug("Regen key validated", "url", reqData.ValidatedURL)
			// if ok, _ := checkUrlOk(reqData.ValidatedURL); !ok {
			// 	http.Error(w, "Could not connect to origin URL", http.StatusBadGateway)
			// 	return
			// }
		}
		if filepath, err := screenshot.Take(&reqData); err == nil {
			serveImage(w, r, filepath, "MISS", "1")
		} else {
			handleServerError(w, err)
		}
		return
	}

	// check database for image
	// var cachedImage database.TemplateImage
	cachedImage, _ := database.GetImage(reqData.UrlKey)

	// has cached image and request url matches cache key for url - return cached image
	if cachedImage.File != "" && cachedImage.CacheKey == reqData.CacheKey {
		slog.Debug("Found cached image", "url", reqData.ValidatedURL, "cache_key", cachedImage.CacheKey)
		serveImage(w, r, filepath.Join(global.ImageDir, cachedImage.File), "HIT", "2")
		return
	}

	// check origin url before using browser
	ok, originOgURL := checkUrlOk(reqData.ValidatedURL)
	if !ok {
		http.Error(w, "Could not connect to origin URL", http.StatusBadGateway)
		return
	}

	// has cached image but origin og url does not match request - return cached image
	originCacheKey := makeCacheKey(originOgURL)
	if cachedImage.File != "" && reqData.CacheKey != originCacheKey {
		slog.Debug("Request image does not match origin", "req", reqData.CacheKey, "origin", originCacheKey)
		serveImage(w, r, global.ImageDir+cachedImage.File, "HIT", "3")
		return
	}

	// generate image.
	// should only get here if:
	// 1. url is not cached at all
	// 2. origin og url matches request (origin updated, our db is stale)
	if filepath, err := screenshot.Take(&reqData); err == nil {
		serveImage(w, r, filepath, "MISS", "0")
	} else {
		handleServerError(w, err)
	}
}

// cleans up old images and url mutexes, sleeps for an hour between cleaning cycles
func cleanup() {
	ticker := time.NewTicker(time.Hour)
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
	return v != "" && (v == global.RegenKey)
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
	if global.AllowedDomainsMap != nil && !global.AllowedDomainsMap[u.Host] {
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
	// Check if the status code is 200 OK
	if ok := resp.StatusCode == http.StatusOK; !ok {
		return false, ""
	}
	// Parse the HTML response
	defer resp.Body.Close()
	doc, err := html.Parse(resp.Body)
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
	params := u.Query()
	params.Del("_regen_")
	if strings.HasPrefix(u.Path, "/template/") {
		return strings.TrimPrefix(u.Path, "/template/") + params.Encode()
	} else {
		return params.Encode()
	}
}

func handleServerError(w http.ResponseWriter, err error) {
	slog.Error("Error serving image", "error", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}
