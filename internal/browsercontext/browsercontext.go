package browsercontext

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
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

func Init() {
	persistBrowser := os.Getenv("PERSIST_BROWSER")
	if persistBrowser == "" {
		persistBrowser = "5m"
	}
	slog.Debug("PERSIST_BROWSER", "value", persistBrowser)
	duration, err := time.ParseDuration(persistBrowser)
	if err != nil {
		log.Fatal(err)
	}
	persistBrowserDuration = duration

	// set up allocator
	cancelAllocator := setUpAllocator()

	// use channel to listen for SIGTERM and SIGINT signals
	// to gracefully shut down the allocator context
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Println("Received signal:", sig)
		cancelAllocator()
		os.Exit(0)
	}()
}

func resetBrowserTimer() {
	slog.Debug("Resetting browser timer", "time", persistBrowserDuration)
	timer.Reset(persistBrowserDuration)
}

func GetTaskContext() (context.Context, context.CancelFunc, func()) {
	if timer == nil {
		timer = time.AfterFunc(persistBrowserDuration, closeBrowser)
	}
	timer.Stop()
	taskCtx, cancel := chromedp.NewContext(getBrowserContext())
	return taskCtx, cancel, resetBrowserTimer
}

// remote uses a straightforward context
func GetRemoteContext() (context.Context, context.CancelFunc) {
	return chromedp.NewContext(allocatorContext)
}

// closes the browser / cancels the browser context
func closeBrowser() {
	browserContextMutex.Lock()
	defer browserContextMutex.Unlock()
	slog.Debug("Terminating browser process")
	cancelBrowserContext()
	browserOpen = false
}

func getBrowserContext() context.Context {
	browserContextMutex.Lock()
	defer browserContextMutex.Unlock()

	if browserOpen {
		return browserContext
	}
	slog.Debug("Launching browser process")
	browserContext, cancelBrowserContext = chromedp.NewContext(allocatorContext)

	if err := chromedp.Run(browserContext); err != nil {
		log.Fatalf("Error creating browser context: %v", err)
	}
	browserOpen = true
	return browserContext
}

func setUpAllocator() (cancel context.CancelFunc) {
	// if remote url
	if IsRemoteBrowser {
		slog.Debug("Creating RemoteAllocator", "url", remoteUrl)
		allocatorContext, cancel = chromedp.NewRemoteAllocator(context.Background(), remoteUrl)
		return cancel
	}

	// if not remote url
	slog.Debug("Creating ExecAllocator")
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("font-render-hinting", "none"),
		chromedp.Flag("disable-font-subpixel-positioning", true),
	)
	font := os.Getenv("FONT_FAMILY")
	if font != "" {
		slog.Debug("Using custom font", "FONT_FAMILY", font)
		opts = append(opts, chromedp.Flag("system-font-family", font))
	}
	allocatorContext, cancel = chromedp.NewExecAllocator(context.Background(), opts...)

	// for testing only
	// var blankOpts []func(*chromedp.ExecAllocator)
	// allocatorContext, cancel = chromedp.NewExecAllocator(context.Background(), blankOpts...)

	return cancel
}
