package browsercontext

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

var remoteUrl = os.Getenv("REMOTE_URL")
var IsRemoteBrowser = remoteUrl != ""

var browserOpen bool
var persistBrowserDuration time.Duration

var allocatorContext context.Context
var browserContext context.Context
var cancelBrowserContext context.CancelFunc

// use mutex to lock access to browserContext while we cancel
// so a perfectly badly timed request will wait
var browserContextMutex = &sync.Mutex{}

var timer *time.Timer

func init() {
	persistBrowser := os.Getenv("PERSIST_BROWSER")
	if persistBrowser == "" {
		persistBrowser = "5m"
	}
	duration, err := time.ParseDuration(persistBrowser)
	if err != nil {
		log.Fatal(err)
	}
	persistBrowserDuration = duration
}

func resetBrowserTimer() {
	timer.Reset(persistBrowserDuration)
}

func GetTaskContext() (context.Context, context.CancelFunc, func()) {
	if IsRemoteBrowser {
		return getAllocatorContext(), cancelBrowserContext, resetBrowserTimer
	}
	if timer == nil {
		timer = time.AfterFunc(persistBrowserDuration, closeBrowser)
	}
	timer.Stop()
	taskCtx, cancel := chromedp.NewContext(getBrowserContext())
	return taskCtx, cancel, resetBrowserTimer
}

// remote uses a straightforward context
func GetRemoteContext() (context.Context, context.CancelFunc) {
	return chromedp.NewContext(getAllocatorContext())
}

// closes the browser / cancels the browser context
func closeBrowser() {
	// log.Println("[DEBUG] Running cleanup")
	browserContextMutex.Lock()
	defer browserContextMutex.Unlock()
	cancelBrowserContext()
	browserOpen = false
}

func getBrowserContext() context.Context {
	browserContextMutex.Lock()
	defer browserContextMutex.Unlock()

	if browserOpen {
		return browserContext
	}
	// log.Println("[DEBUG] Creating new browser context from allocator context")

	browserContext, cancelBrowserContext = chromedp.NewContext(getAllocatorContext())

	// log.Println("[DEBUG] Running first tab to create browser context")
	if err := chromedp.Run(browserContext); err != nil {
		log.Fatalf("[ERROR] Error creating browser context: %v", err)
	}
	browserOpen = true
	return browserContext
}

func getAllocatorContext() context.Context {
	if allocatorContext != nil {
		return allocatorContext
	}

	// log.Println("[DEBUG] Creating new allocator context")
	// if remote url
	if IsRemoteBrowser {
		// log.Printf("[DEBUG] Creating remote allocator context for %v", remoteUrl)
		allocatorContext, _ = chromedp.NewRemoteAllocator(context.Background(), remoteUrl)
		return allocatorContext
	}

	// if not remote url
	// log.Println("[DEBUG] Creating new executor allocator context")

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("font-render-hinting", "none"),
		chromedp.Flag("disable-font-subpixel-positioning", true),
	)
	font := os.Getenv("FONT_FAMILY")
	if font != "" {
		opts = append(opts, chromedp.Flag("system-font-family", font))
	}
	allocatorContext, _ = chromedp.NewExecAllocator(context.Background(), opts...)

	// for testing only
	// var blankOpts []func(*chromedp.ExecAllocator)
	// allocatorContext, _ = chromedp.NewExecAllocator(context.Background(), blankOpts...)

	// this is necessary to reuse one instance of the browser
	var allocatorChildContext context.Context
	allocatorChildContext, _ = chromedp.NewContext(getAllocatorContext())

	return allocatorChildContext
}
