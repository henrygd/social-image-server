package screenshot

import (
	"context"
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
	"github.com/henrygd/social-image-server/internal/database"
	"github.com/henrygd/social-image-server/internal/global"
	"github.com/henrygd/social-image-server/internal/templates"
)

func getViewportDimensions(params *url.Values) (viewportWidth int64, viewportHeight int64, scale float64) {
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
	// force facebook's recommended 1200/630 aspect ratio
	viewportHeight = int64((float64(viewportWidth) * 0.525))
	// calculate scale to make image 2000px wide
	scale = global.ImageOptions.Width / float64(viewportWidth)
	return viewportWidth, viewportHeight, scale
}

func getDelay(params *url.Values) (delay int64) {
	paramDelay := params.Get("delay")
	if paramDelay != "" {
		delay, _ = strconv.ParseInt(paramDelay, 10, 64)
		if delay > 10000 {
			delay = 10000
		}
	}
	return delay
}

func getImageFormat(params *url.Values) (imageFormat string, imageExtension string) {
	paramFormat := params.Get("format")
	if paramFormat == "png" {
		return "png", ".png"
	}
	if paramFormat == "jpeg" {
		return "jpeg", ".jpg"
	}
	return global.ImageOptions.Format, global.ImageOptions.Extension
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
func takeScreenshot(validatedUrl string, params *url.Values) (filepath string, err error) {
	imageOptions := &global.ImageOptions

	viewportWidth, viewportHeight, scale := getViewportDimensions(params)
	delay := getDelay(params)
	imageFormat, imageExtension := getImageFormat(params)

	// get context
	taskCtx, cancel, resetBrowserTimer := getContext()
	defer cancel()
	if resetBrowserTimer != nil {
		defer resetBrowserTimer()
	}

	// create file for screenshot
	f, err := os.CreateTemp(global.ImageDir, "*"+imageExtension)
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
	)
	// add delay
	if delay != 0 {
		tasks = append(tasks, chromedp.Sleep(time.Duration(delay)*time.Millisecond))
	}
	// take screenshot
	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		format := page.CaptureScreenshotFormat(imageFormat)
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

// Generates a screenshot of a URL.
func Take(req *global.ReqData) (filepath string, err error) {
	if req.Template == "" {
		slog.Debug("Taking screenshot", "url", req.ValidatedURL)
		req.ValidatedURL += "?og-image-request=true"
		filepath, err = takeScreenshot(req.ValidatedURL, &req.Params)
	}

	// if requesting template, start temp server for the screenshot
	if req.Template != "" {
		slog.Debug("Taking screenshot", "template", req.Template)
		var server *http.Server
		var serverURL string
		server, serverURL, err = templates.TempServer(req.Template)
		if err != nil {
			return "", err
		}
		defer server.Close()
		defer slog.Debug("Template server stopped", "template", req.Template)
		serverURL += "?" + req.Params.Encode()
		filepath, err = takeScreenshot(serverURL, &req.Params)
	}

	if err != nil {
		return "", err
	}

	// add image to database
	err = database.AddImage(&database.Image{
		Url:      req.UrlKey,
		File:     strings.TrimPrefix(filepath, global.ImageDir),
		CacheKey: req.CacheKey,
	})
	if err != nil {
		return "", err
	}

	return filepath, nil
}
