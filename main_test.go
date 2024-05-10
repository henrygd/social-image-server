package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var mockServer *httptest.Server

var cacheKey = "abcdef123456"
var ogImageRequestParam string

func TestMain(m *testing.M) {
	mockServer = createMockServer()
	defer mockServer.Close()
	log.Println("Starting mock server on", mockServer.URL)
	serverUrl, _ := url.Parse(mockServer.URL)
	dataDir := filepath.Join(os.TempDir(), "social-image-server-test")
	os.Setenv("ALLOWED_DOMAINS", serverUrl.Host)
	os.Setenv("DATA_DIR", dataDir)
	os.Setenv("REGEN_KEY", "jamesconnolly")
	// Run the tests
	code := m.Run()
	// Clean up after tests
	os.RemoveAll(dataDir)
	// Exit with the test code
	os.Exit(code)
}

func createMockServer() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			if r.URL.Query().Has("og-image-request") {
				ogImageRequestParam = r.URL.Query().Get("og-image-request")
			}
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "text/html")
			htmlContent := `<html><head><title>valid</title></head><body>valid</body></html>`
			w.Write([]byte(htmlContent))
			return
		}
		if r.URL.Path == "/cachekey" {
			ogImageUrl := fmt.Sprintf("https://test.com/get?url=%s/cachekey&cache_key=%s", r.Host, cacheKey)
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

func TestEndpoints(t *testing.T) {
	router := setUpRouter()

	// Test cases
	testCases := []struct {
		name            string
		url             string
		expectedCode    int
		expectedBody    string
		expectedImage   bool
		expectedOgCache string
		expectedOgCode  string
		newCacheKey     string
	}{
		{
			name:            "No URL",
			url:             "/get",
			expectedCode:    http.StatusBadRequest,
			expectedBody:    "no url supplied\n",
			expectedImage:   false,
			expectedOgCache: "",
			expectedOgCode:  "",
		},
		{
			name:            "Invalid URL",
			url:             "/get?url=lkj laskd",
			expectedCode:    http.StatusBadRequest,
			expectedBody:    "invalid url\n",
			expectedImage:   false,
			expectedOgCache: "",
			expectedOgCode:  "",
		},
		{
			name:            "404 URL",
			url:             fmt.Sprintf("/get?url=%s/invalid", mockServer.URL),
			expectedCode:    http.StatusNotFound,
			expectedBody:    "Requested URL not found\n",
			expectedImage:   false,
			expectedOgCache: "",
			expectedOgCode:  "",
		},
		{
			name:            "Domain not allowed",
			url:             "/get?url=nytimes.com",
			expectedCode:    http.StatusBadRequest,
			expectedBody:    "domain nytimes.com not allowed\n",
			expectedImage:   false,
			expectedOgCache: "",
			expectedOgCode:  "",
		},
		{
			name:            "Valid URL",
			url:             fmt.Sprintf("/get?url=%s", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "0",
		},
		{
			name:            "Cached Image",
			url:             fmt.Sprintf("/get?url=%s&width=1200", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "2",
		},
		{
			name:            "Cache key BAD (no cache)",
			url:             fmt.Sprintf("/get?url=%s/cachekey&cache_key=12345", mockServer.URL),
			expectedCode:    http.StatusBadRequest,
			expectedBody:    "request cache_key does not match origin cache_key\n",
			expectedImage:   false,
			expectedOgCache: "",
			expectedOgCode:  "",
		},
		{
			name:            "Cache key GOOD (no cache)",
			url:             fmt.Sprintf("/get?url=%s/cachekey&cache_key=abcdef123456", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "0",
		},
		{
			name:            "Cache key GOOD (cached)",
			url:             fmt.Sprintf("/get?url=%s/cachekey&cache_key=abcdef123456", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "2",
		},
		{
			name:            "Cache key BAD (has cache)",
			url:             fmt.Sprintf("/get?url=%s/cachekey&cache_key=12345", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "3",
		},
		{
			name:            "Cache key CHANGE",
			url:             fmt.Sprintf("/get?url=%s/cachekey&cache_key=987654321", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "0",
			newCacheKey:     "987654321",
		},
		{
			name:            "Cache key OLD (has cache)",
			url:             fmt.Sprintf("/get?url=%s/cachekey&cache_key=abcdef123456", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "3",
		},
		{
			name:            "Regen Param (bad value)",
			url:             fmt.Sprintf("/get?url=%s&_regen_=12345", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "2",
		},
		{
			name:            "Regen Param (good value)",
			url:             fmt.Sprintf("/get?url=%s&_regen_=jamesconnolly", mockServer.URL),
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "1",
		},
	}

	var imageProcessingTimes []int64

	for _, tc := range testCases {
		if tc.newCacheKey != "" {
			cacheKey = tc.newCacheKey
		}

		t.Run(tc.name, func(t *testing.T) {
			var start time.Time

			req, err := http.NewRequest("GET", tc.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			if tc.expectedImage {
				start = time.Now()
			}

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedCode, rr.Code)

			if tc.expectedImage {
				duration := time.Since(start).Milliseconds()
				imageProcessingTimes = append(imageProcessingTimes, duration)
			}

			if tc.expectedImage {
				assert.Equal(t, "image/jpeg", rr.Header().Get("Content-Type"))
				assert.Equal(t, tc.expectedOgCode, rr.Header().Get("x-og-code"))
				assert.Equal(t, tc.expectedOgCache, rr.Header().Get("x-og-cache"))
			} else {
				assert.Equal(t, tc.expectedBody, rr.Body.String())
			}
		})
	}

	t.Run("Sends URL param og-image-request to origin", func(t *testing.T) {
		assert.Equal(t, "true", ogImageRequestParam)
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

	// todo: same url mutex test, CACHE_TIME test
}
