package screenshot

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/henrygd/social-image-server/internal/browsercontext"
	"github.com/henrygd/social-image-server/internal/database"
	"github.com/henrygd/social-image-server/internal/global"
)

func getViewportDimensions(params url.Values) (viewportWidth int64, viewportHeight int64, scale float64) {
	paramWidth := params.Get("width")
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
	scale = global.ImageOptions.Width / float64(viewportWidth)
	return viewportWidth, viewportHeight, scale
}

func getDelay(params url.Values) (delay int64) {
	paramDelay := params.Get("delay")
	if paramDelay != "" {
		delay, _ = strconv.ParseInt(paramDelay, 10, 64)
		if delay > 10000 {
			delay = 10000
		}
	}
	return delay
}

// getContext checks if the browser is remote or local and gets the corresponding context and functions.
func getContext() (taskCtx context.Context, cancel context.CancelFunc, resetBrowserTimer func()) {
	if browsercontext.IsRemoteBrowser {
		// if using remote browser, use remote context
		taskCtx, cancel = browsercontext.GetRemoteContext()
	} else {
		taskCtx, cancel, resetBrowserTimer = browsercontext.GetTaskContext()
	}
	return taskCtx, cancel, resetBrowserTimer
}

// takeScreenshot takes a screenshot of a webpage.
//
// It accepts the validated URL as a string and parameters for the screenshot.
// Returns the filepath of the saved screenshot and any error encountered.
func takeScreenshot(validatedUrl string, params url.Values) (filepath string, err error) {
	imageOptions := &global.ImageOptions

	// get viewport dimensions
	viewportWidth, viewportHeight, scale := getViewportDimensions(params)

	// get delay
	delay := getDelay(params)

	// get context
	taskCtx, cancel, resetBrowserTimer := getContext()
	defer cancel()
	if resetBrowserTimer != nil {
		defer resetBrowserTimer()
	}

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

	return filepath, nil
}

// Url generates a screenshot of a URL.
//
// Parameters:
// - validatedUrl: the validated URL for the screenshot
// - urlKey: the key associated with the URL
// - pageCacheKey: the cache key for the page
// - params: additional parameters for the screenshot
// Returns the file path where the screenshot is saved and any error encountered.
func Url(validatedUrl string, urlKey string, pageCacheKey string, params url.Values) (filepath string, err error) {
	slog.Debug("Taking screenshot", "url", validatedUrl)

	validatedUrl += "?og-image-request=true"

	filepath, err = takeScreenshot(validatedUrl, params)
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

// Template generates a screenshot of a template.
//
// serverUrl: the URL of the server serving template
// params: additional parameters for the screenshot
// Returns the file path where the screenshot is saved and any error encountered.
func Template(serverUrl string, params url.Values) (filepath string, err error) {
	// add params to serverUrl
	serverUrl += "?" + params.Encode()
	filepath, err = takeScreenshot(serverUrl, params)
	if err != nil {
		return "", err
	}
	return filepath, nil
}
