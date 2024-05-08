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
	"github.com/henrygd/social-image-server/internal/scraper"
)

var dataDir = os.Getenv("DATA_DIR")
var remoteUrl = os.Getenv("REMOTE_URL")
var allowedDomains = os.Getenv("ALLOWED_DOMAINS")
var allowedDomainsMap = make(map[string]bool)

func main() {
	if dataDir == "" {
		dataDir = "./data"
	}
	imgDir := dataDir + "/images"
	// create folders
	err := os.MkdirAll(imgDir, 0755)
	if err != nil {
		log.Fatal(err)
	}
	err = os.MkdirAll(dataDir+"/db", 0755)
	if err != nil {
		log.Fatal(err)
	}

	// create database
	err = database.Init(dataDir)
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
	var globalContext context.Context
	var cancel context.CancelFunc
	if remoteUrl != "" {
		globalContext, cancel = chromedp.NewRemoteAllocator(context.Background(), remoteUrl)
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
		globalContext, cancel = chromedp.NewExecAllocator(context.Background(), opts...)
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
		mutex := concurrency.GetOrCreateUrlMutex(urlKey)
		mutex.Lock()
		defer mutex.Unlock()

		regen := params.Get("_regen_") != "" && (params.Get("_regen_") == os.Getenv("REGEN_KEY"))

		if regen {
			// if regen key is provided, delete image from database
			err = database.DeleteImage(imgDir, urlKey)
			if err != nil {
				handleServerError(w, err)
				return
			}
		} else {
			// else check database for cached image
			img, err := database.GetImage(urlKey)
			if err == nil {
				serveImage(w, r, imgDir+img.File)
				return
			}
		}

		// set viewport dimensions
		supplied_width := params.Get("width")
		var viewportWidth int64
		var viewportHeight int64
		if supplied_width != "" {
			viewportWidth, _ = strconv.ParseInt(supplied_width, 10, 64)
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
		supplied_delay := params.Get("delay")
		var delay int64
		if supplied_delay != "" {
			delay, _ = strconv.ParseInt(supplied_delay, 10, 64)
			if delay > 10000 {
				delay = 10000
			}
		}

		suppliedCacheKey := params.Get("cache_key")

		// if cache key set
		// if cacheKey != "" {
		// 	cacheKey = uuid.New().String()
		// }

		// check that url provides 200 response before generating image
		ok, pageCacheKey := checkUrlOk(validatedUrl, suppliedCacheKey)
		if !ok {
			http.Error(w, "Requested URL not found", http.StatusNotFound)
			return
		}

		log.Println("suppliedCacheKey", suppliedCacheKey)
		log.Println("pageCacheKey", pageCacheKey)

		// add og-image-request parameter to url
		validatedUrl += "?og-image-request=true"

		// create task context
		taskCtx, cancel := chromedp.NewContext(globalContext)
		defer cancel()

		tasks := chromedp.Tasks{}

		// set prefers dark mode
		if params.Get("dark") == "true" {
			tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
				emulatedMedia := emulation.SetEmulatedMedia()
				emulatedMedia.Features = append(emulatedMedia.Features, &emulation.MediaFeature{Name: "prefers-color-scheme", Value: "dark"})
				err = emulatedMedia.Do(ctx)
				return err
			}))
		}

		// navigate to url and capture screenshot to buf
		var buf = make([]byte, 0, 200*1024)
		tasks = append(tasks,
			// chromedp.Emulate(device.IPad),
			chromedp.EmulateViewport(viewportWidth, viewportHeight, chromedp.EmulateScale(scale)),
			// set prefers dark mode
			chromedp.Navigate(validatedUrl),
			// chromedp.Evaluate(`document.documentElement.style.overflow = 'hidden'`, nil),
			chromedp.Sleep(time.Duration(delay)*time.Millisecond),
			chromedp.CaptureScreenshot(&buf))

		if err = chromedp.Run(taskCtx, tasks); err != nil {
			handleServerError(w, err)
			return
		}

		// save image
		f, err := os.CreateTemp(imgDir, "*.jpg")
		if err != nil {
			log.Fatal(err)
		}
		filepath := f.Name()

		// decode the png
		img, err := png.Decode(bytes.NewReader(buf))
		if err != nil {
			handleServerError(w, err)
			return
		}
		// encode the jpeg
		buff := new(bytes.Buffer)
		err = jpeg.Encode(buff, img, &jpeg.Options{Quality: 90})
		if err != nil {
			handleServerError(w, err)
			return
		}
		// write image to file
		err = os.WriteFile(filepath, buff.Bytes(), 0o644)
		if err != nil {
			handleServerError(w, err)
			return
		}
		// add image to database
		if _, err := database.AddImage(&database.SocialImage{
			Url:  urlKey,
			File: strings.TrimPrefix(filepath, imgDir),
		}); err != nil {
			handleServerError(w, err)
			return
		}

		serveImage(w, r, filepath)
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

// cleans up old images and url mutexes, sleeps for an hour between cleaning cycles
func cleanup() {
	for {
		err := database.Clean(dataDir)
		if err != nil {
			log.Println(err)
		}
		time.Sleep(time.Hour)
		concurrency.CleanUrlMutexes(time.Now())
	}
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

// Check if the status code of a url is 200 OK
func checkUrlOk(validatedUrl string, suppliedCacheKey string) (bool, string) {
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
	ok := resp.StatusCode == http.StatusOK

	// return if no cache key
	if suppliedCacheKey == "" {
		return ok, ""
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
