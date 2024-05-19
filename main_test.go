package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/henrygd/social-image-server/internal/database"
	"github.com/henrygd/social-image-server/internal/global"
	"github.com/stretchr/testify/assert"
)

var mockServer *httptest.Server

var regenKey = "jamesconnolly"
var requestParams string
var mockOgImageURL string
var dataDir string
var imageProcessingTimes []int64

func TestMain(m *testing.M) {
	mockServer = createMockServer()
	defer mockServer.Close()
	mockOgImageURL = "/capture?url=" + mockServer.URL
	log.Println("Starting mock server on", mockServer.URL)
	dataDir = filepath.Join(os.TempDir(), "social-image-server-test")
	mockServerURL, _ := url.Parse(mockServer.URL)
	os.Setenv("ALLOWED_DOMAINS", mockServerURL.Host)
	os.Setenv("DATA_DIR", dataDir)
	os.Setenv("REGEN_KEY", regenKey)
	os.Setenv("IMG_WIDTH", "1000")
	os.Setenv("LOG_LEVEL", "debug")
	// Run the tests
	code := m.Run()
	// Clean up after tests
	os.RemoveAll(dataDir)
	// Exit with the test code
	os.Exit(code)
}

func createMockServer() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/template-test" {
			requestParams = r.URL.Query().Encode()
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "text/html")
			htmlContent := `<html><head><title>valid</title>
			<meta property="og:image" content="https://example.com` + mockOgImageURL + `" />
			</head><body>valid</body></html>`
			w.Write([]byte(htmlContent))
			return
		}
		if r.URL.Path == "/about" {
			ogImageUrl := fmt.Sprintf("https://example.com/capture?url=%s/about&width=1200", mockServer.URL)
			htmlContent := `
			<html><head><title>valid site</title>
			<meta property="og:image" content="` + ogImageUrl + `" />
			</head><body>valid site</body></html>`
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(htmlContent))
			return
		}
		http.NotFound(w, r)
	}))
	return server
}

type testCase struct {
	name            string
	url             string
	expectedCode    int
	expectedBody    string
	expectedImage   bool
	expectedOgCache string
	expectedOgCode  string
	newMockOgURL    string
	maxReqTime      time.Duration
	minReqTime      time.Duration
}

func TestCapture(t *testing.T) {
	router := setUpRouter()

	// Test cases
	testCases := []testCase{
		{
			name:          "No URL",
			url:           "/capture",
			expectedCode:  http.StatusBadRequest,
			expectedBody:  "no url supplied\n",
			expectedImage: false,
		},
		{
			name:          "Invalid URL",
			url:           "/capture?url=lkj laskd",
			expectedCode:  http.StatusBadRequest,
			expectedBody:  "invalid url\n",
			expectedImage: false,
		},
		{
			name:          "Bad origin URL",
			url:           fmt.Sprintf("/capture?url=%s/invalid", mockServer.URL),
			expectedCode:  http.StatusBadGateway,
			expectedBody:  "Could not connect to origin URL\n",
			expectedImage: false,
		},
		{
			name:          "Domain not allowed",
			url:           "/capture?url=nytimes.com",
			expectedCode:  http.StatusBadRequest,
			expectedBody:  "domain nytimes.com not allowed\n",
			expectedImage: false,
		},
		{
			name:            "Takes screenshot",
			url:             fmt.Sprintf("/capture?url=%s", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "0",
		},
		{
			name:            "Cached image",
			url:             fmt.Sprintf("/capture?url=%s", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "2",
		},
		{
			name:            "/get route works correctly",
			url:             fmt.Sprintf("/get?url=%s", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "2",
		},
		{
			name:            "Cache key does not match db key or origin key - return cached image",
			url:             fmt.Sprintf("/capture?url=%s?width=900", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "3",
		},
		{
			name:            "Mismatched cache key allowed if not in database - generate",
			url:             fmt.Sprintf("/capture?url=%s/about?width=900", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "0",
		},
		{
			name:            "Cache key does not match db key or origin key - return cached image",
			url:             fmt.Sprintf("/capture?url=%s/about&width=1100", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "3",
		},
		{
			name:            "About page cache key does not match db key but does match origin - regenerate",
			url:             fmt.Sprintf("/capture?url=%s/about&width=1200", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "0",
		},
		{
			name:            "Cached image on about page",
			url:             fmt.Sprintf("/capture?url=%s/about&width=1200", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "2",
		},
		{
			name:            "Home page cache key does not match db key but does match origin key - regenerate",
			url:             fmt.Sprintf("/capture?url=%s&key=testkey", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "0",
			newMockOgURL:    fmt.Sprintf("/capture?url=%s&key=testkey", mockServer.URL),
			maxReqTime:      time.Second,
		},
		{
			name:            "Params removed from origin url",
			url:             fmt.Sprintf("/capture?url=%s", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "0",
			newMockOgURL:    fmt.Sprintf("/capture?url=%s", mockServer.URL),
			maxReqTime:      time.Second,
		},
		{
			name:            "Regen Param good value - regenerate",
			url:             fmt.Sprintf("/capture?url=%s/about&width=1200&_regen_=%s", mockServer.URL, regenKey),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "1",
		},
		{
			name:            "Regen Param bad value - return cached image",
			url:             fmt.Sprintf("/capture?url=%s/about&width=1200&_regen_=margaretthatcher", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "2",
		},
		{
			name:            "Delay param",
			url:             fmt.Sprintf("/capture?url=%s&delay=1000&_regen_=%s", mockServer.URL, regenKey),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "1",
			minReqTime:      time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runTest(t, tc, router)
		})
	}

	t.Run("Sends URL param og-image-request to origin", func(t *testing.T) {
		assert.Contains(t, requestParams, "og-image-request=true")
	})

	var img image.Image
	var imgOneContentLength int64
	t.Run("Default format is jpeg", func(t *testing.T) {
		req, err := http.NewRequest("GET", fmt.Sprintf("/capture?url=%s&_regen_=%s", mockServer.URL, regenKey), nil)
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		assert.Equal(t, "image/jpeg", rr.Header().Get("Content-Type"))
		assert.Equal(t, "1", rr.Header().Get("x-og-code"))
		assert.Equal(t, "MISS", rr.Header().Get("x-og-cache"))
		// convert body to jpeg to use for width test
		img, _ = jpeg.Decode(bytes.NewReader(rr.Body.Bytes()))
		imgOneContentLength, _ = strconv.ParseInt(rr.Header().Get("Content-Length"), 10, 64)
	})

	t.Run("Format param", func(t *testing.T) {
		req, err := http.NewRequest("GET", fmt.Sprintf("/capture?url=%s&_regen_=%s&format=png", mockServer.URL, regenKey), nil)
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		assert.Equal(t, "image/png", rr.Header().Get("Content-Type"))
		assert.Equal(t, "1", rr.Header().Get("x-og-code"))
		assert.Equal(t, "MISS", rr.Header().Get("x-og-cache"))
	})

	t.Run("IMG_WIDTH", func(t *testing.T) {
		assert.Equal(t, 1000, img.Bounds().Dx())
	})

	t.Run("IMG_QUALITY", func(t *testing.T) {
		os.Setenv("IMG_QUALITY", "50")
		defer os.Unsetenv("IMG_QUALITY")
		nRouter := setUpRouter()
		req, err := http.NewRequest("GET", fmt.Sprintf("/capture?url=%s&_regen_=%s", mockServer.URL, regenKey), nil)
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		nRouter.ServeHTTP(rr, req)
		assert.Equal(t, "image/jpeg", rr.Header().Get("Content-Type"))
		assert.Equal(t, "1", rr.Header().Get("x-og-code"))
		assert.Equal(t, "MISS", rr.Header().Get("x-og-cache"))
		contentLength, _ := strconv.ParseInt(rr.Header().Get("Content-Length"), 10, 64)
		assert.Less(t, contentLength, imgOneContentLength)
	})

	t.Run("IMG_FORMAT", func(t *testing.T) {
		os.Setenv("IMG_FORMAT", "png")
		defer os.Unsetenv("IMG_FORMAT")
		nRouter := setUpRouter()
		req, err := http.NewRequest("GET", fmt.Sprintf("/capture?url=%s&_regen_=%s", mockServer.URL, regenKey), nil)
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		nRouter.ServeHTTP(rr, req)
		assert.Equal(t, "image/png", rr.Header().Get("Content-Type"))
		assert.Equal(t, "1", rr.Header().Get("x-og-code"))
		assert.Equal(t, "MISS", rr.Header().Get("x-og-cache"))
	})
}

func TestTemplate(t *testing.T) {
	os.Setenv("IMG_FORMAT", "png")
	defer os.Unsetenv("IMG_FORMAT")
	router := setUpRouter()

	// Create valid template
	os.Mkdir(filepath.Join(dataDir, "templates", "valid-template"), 0755)
	file, _ := os.Create(filepath.Join(dataDir, "templates", "valid-template", "index.html"))
	file.WriteString(`
		<html>
			<body>
				<h1>hello <span>world</span></h1>
				<script>
					document.querySelector('span').innerText = new URLSearchParams(location.search).get('name')
				</script>
			</body>
		</html>`,
	)

	// Test cases
	testCases := []testCase{
		{
			name:          "Invalid template",
			url:           "/template/invalid-name",
			expectedCode:  http.StatusBadRequest,
			expectedBody:  "Invalid template\n",
			expectedImage: false,
		},
		{
			name:          "No URL",
			url:           "/template/valid-template",
			expectedCode:  http.StatusBadRequest,
			expectedBody:  "no url supplied\n",
			expectedImage: false,
		},
		{
			name:          "Bad origin URL",
			url:           fmt.Sprintf("/capture?url=%s/invalid", mockServer.URL),
			expectedCode:  http.StatusBadGateway,
			expectedBody:  "Could not connect to origin URL\n",
			expectedImage: false,
		},
		{
			name:          "Domain not allowed",
			url:           "/template/valid-template/?url=nytimes.com",
			expectedCode:  http.StatusBadRequest,
			expectedBody:  "domain nytimes.com not allowed\n",
			expectedImage: false,
		},
		{
			name:            "Takes screenshot",
			url:             fmt.Sprintf("/template/valid-template/?url=%s", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCode:  "0",
			expectedOgCache: "MISS",
			newMockOgURL:    fmt.Sprintf("/template/valid-template/?url=%s", mockServer.URL),
		},
		{
			name:            "Cached image",
			url:             fmt.Sprintf("/template/valid-template/?url=%s", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "2",
		},
		{
			name:            "Cache key does not match db key or origin key - return cached image",
			url:             fmt.Sprintf("/template/valid-template/?url=%s&title=hello", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "3",
		},
		{
			name:            "Mismatched cache key allowed if not in database - generate",
			url:             fmt.Sprintf("/template/valid-template/?url=%s/template-test&title=hello", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "0",
		},
		{
			name:            "Cache key does not match db key or origin key - return cached image",
			url:             fmt.Sprintf("/template/valid-template/?url=%s/template-test&title=hola", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "3",
		},
		{
			name:            "Cache key does not match db key but does match origin - regenerate",
			url:             fmt.Sprintf("/template/valid-template/?url=%s/template-test&title=moshimoshi", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "0",
			newMockOgURL:    fmt.Sprintf("/template/valid-template/?url=%s/template-test&title=moshimoshi", mockServer.URL),
		},
		{
			name:            "Cached image on about page",
			url:             fmt.Sprintf("/template/valid-template/?url=%s/template-test&title=moshimoshi", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "2",
		},
		{
			name:            "Regen Param good value - regenerate",
			url:             fmt.Sprintf("/template/valid-template/?url=%s&title=nihao&_regen_=%s", mockServer.URL, regenKey),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "1",
		},
		{
			name:            "Regen Param bad value - return cached image",
			url:             fmt.Sprintf("/template/valid-template/?url=%s&title=nihao&_regen_=margaretthatcher", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "2",
		},
		{
			name:            "Params removed from origin url",
			url:             fmt.Sprintf("/template/valid-template/?url=%s", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "0",
			newMockOgURL:    fmt.Sprintf("/template/valid-template/?url=%s", mockServer.URL),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runTest(t, tc, router)
		})
	}

	t.Run("Passes query params to template", func(t *testing.T) {
		// earth and earth should have same content length
		// jupiter and earth should have different content length
		names := []string{"earth", "earth", "jupiter"}
		var imageLengths []int64

		for _, name := range names {
			url := fmt.Sprintf("/template/valid-template/?url=%s&name=%s&_regen_=%s", mockServer.URL, name, regenKey)
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				t.Fatal(err)
			}
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			assert.Equal(t, "image/png", rr.Header().Get("Content-Type"))
			assert.Equal(t, "1", rr.Header().Get("x-og-code"))
			assert.Equal(t, "MISS", rr.Header().Get("x-og-cache"))
			length, _ := strconv.ParseInt(rr.Header().Get("Content-Length"), 10, 64)
			imageLengths = append(imageLengths, length)
		}
		assert.Equal(t, imageLengths[0], imageLengths[1])
		assert.NotEqual(t, imageLengths[0], imageLengths[2])
	})

	// note on cache time - this test verifys that the cache time is working
	// however, we only run database.Clean() once an hour, so functional min time is 1 hour
	t.Run("CACHE_TIME", func(t *testing.T) {
		defer os.Unsetenv("CACHE_TIME")
		// with default cache time, clean should not delete any files
		initialImageNum := filesInDir(global.ImageDir)
		assert.Greater(t, initialImageNum, 0)
		database.Clean()
		imageNum := filesInDir(global.ImageDir)
		assert.Equal(t, initialImageNum, imageNum)
		// sleep for just over 1 second
		time.Sleep(time.Millisecond * 1100)
		// with cache time 5 seconds, there should still be files
		os.Setenv("CACHE_TIME", "5 seconds")
		database.Clean()
		imageNum = filesInDir(global.ImageDir)
		assert.Greater(t, imageNum, 0)
		// with cache time 1 second, the files should be cleaned up
		os.Setenv("CACHE_TIME", "1 second")
		database.Clean()
		imageNum = filesInDir(global.ImageDir)
		assert.Equal(t, imageNum, 0)
	})

	// the first image request should take longest since it needs to open browser
	t.Run("First image generation is longest", func(t *testing.T) {
		var longestTime int64 = 0
		for _, time := range imageProcessingTimes {
			if time > longestTime {
				longestTime = time
			}
		}
		assert.Equal(t, imageProcessingTimes[0], longestTime)
	})
}

func runTest(t testing.TB, tc testCase, router *http.ServeMux) {
	t.Helper()

	if tc.newMockOgURL != "" {
		mockOgImageURL = tc.newMockOgURL
	}

	req, err := http.NewRequest("GET", tc.url, nil)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, tc.expectedCode, rr.Code)

	if tc.maxReqTime != 0 {
		assert.Less(t, time.Since(start), tc.maxReqTime)
	} else if tc.minReqTime != 0 {
		assert.Greater(t, time.Since(start), tc.minReqTime)
	}

	if tc.expectedImage {
		if tc.minReqTime == 0 {
			imageProcessingTimes = append(imageProcessingTimes, time.Since(start).Milliseconds())
		}
		assert.Contains(t, rr.Header().Get("Content-Type"), "image/")
		assert.Equal(t, tc.expectedOgCode, rr.Header().Get("x-og-code"))
		assert.Equal(t, tc.expectedOgCache, rr.Header().Get("x-og-cache"))
	} else {
		assert.Equal(t, tc.expectedBody, rr.Body.String())
	}
}

// returns the number of files in the given directory
func filesInDir(dir string) int {
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	return len(files)
}
