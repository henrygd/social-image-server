package browsercontext

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/chromedp/chromedp"
)

var remoteUrl = os.Getenv("REMOTE_URL")
var maxTabs = 5
var openTabs chan struct{}
var isRemoteBrowser bool

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
	isRemoteBrowser = remoteUrl != ""
	// set up max tabs
	if tabs, ok := os.LookupEnv("MAX_TABS"); ok {
		var err error
		maxTabs, err = strconv.Atoi(tabs)
		if err != nil || maxTabs < 1 {
			slog.Error("Invalid MAX_TABS", "value", tabs, "min", 1)
			os.Exit(1)
		}
	}
	slog.Debug("MAX_TABS", "value", maxTabs)
	openTabs = make(chan struct{}, maxTabs)
	// set up persist browser time
	persistBrowser := os.Getenv("PERSIST_BROWSER")
	if persistBrowser == "" {
		persistBrowser = "5m"
	}
	duration, err := time.ParseDuration(persistBrowser)
	if err != nil {
		slog.Error(err.Error(), "PERSIST_BROWSER", persistBrowser)
		os.Exit(1)
	}
	slog.Debug("PERSIST_BROWSER", "value", persistBrowser)
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

func TaskCleanup() {
	// decrement open tabs
	<-openTabs
	// if exec allocator, reset timer
	if !isRemoteBrowser {
		slog.Debug("Resetting browser timer", "time", persistBrowserDuration)
		timer.Reset(persistBrowserDuration)
	}
}

// creates and returns a new browser context (tab)
func GetTaskContext() (context.Context, context.CancelFunc) {
	// increment tabs / block if already at max tabs until space in channel
	openTabs <- struct{}{}
	if isRemoteBrowser {
		// remote uses a straightforward context
		return chromedp.NewContext(allocatorContext)
	}
	// if not remote, stop timer and use existing exec browser context
	if timer == nil {
		timer = time.AfterFunc(persistBrowserDuration, closeBrowser)
	}
	timer.Stop()
	return chromedp.NewContext(getBrowserContext())
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
	if isRemoteBrowser {
		slog.Debug("Creating RemoteAllocator", "url", remoteUrl)
		allocatorContext, cancel = chromedp.NewRemoteAllocator(context.Background(), remoteUrl)
		return cancel
	}

	// if not remote url
	slog.Debug("Creating ExecAllocator")
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("font-render-hinting", "none"),
		chromedp.Flag("disable-font-subpixel-positioning", true),
		chromedp.Flag("audio", false),
		// chromedp.Flag("max-gum-fps", "30"),
	)
	font := os.Getenv("FONT_FAMILY")
	if font != "" {
		slog.Debug("Using custom font", "FONT_FAMILY", font)
		opts = append(opts, chromedp.Flag("system-font-family", font))
	}
	allocatorContext, cancel = chromedp.NewExecAllocator(context.Background(), opts...)

	// non-headless for testing only
	// var blankOpts []func(*chromedp.ExecAllocator)
	// blankOpts = append(blankOpts, chromedp.Flag("hide-scrollbars", true), chromedp.Flag("audio", false))
	// allocatorContext, cancel = chromedp.NewExecAllocator(context.Background(), blankOpts...)

	return cancel
}
