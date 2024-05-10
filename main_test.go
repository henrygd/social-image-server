package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	dataDir := filepath.Join(os.TempDir(), "social-image-server-test")
	os.Setenv("DATA_DIR", dataDir)
	os.Setenv("REGEN_KEY", "jamesconnolly")
	os.Setenv("ALLOWED_DOMAINS", "henrygd.me,democrats.org")
	// Run the tests
	code := m.Run()
	// Clean up after tests
	os.RemoveAll(dataDir)
	// Exit with the test code
	os.Exit(code)
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
			url:             "/get?url=democrats.org/ceasefire",
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
			name:            "Request key doesn't match (no cache)",
			url:             "/get?url=henrygd.me&cache_key=12345",
			expectedCode:    http.StatusBadRequest,
			expectedBody:    "request cache_key does not match origin cache_key\n",
			expectedImage:   false,
			expectedOgCache: "",
			expectedOgCode:  "",
		},
		{
			name:            "Valid URL",
			url:             "/get?url=henrygd.me",
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "0",
		},
		{
			name:            "Cached Image",
			url:             "/get?url=henrygd.me&width=1200",
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "2",
		},
		{
			name:            "Request key doesn't match (has cache)",
			url:             "/get?url=henrygd.me&cache_key=12345",
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "3",
		},
		{
			name:            "Regen Param (bad value)",
			url:             "/get?url=henrygd.me&_regen_=12345",
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "HIT",
			expectedOgCode:  "2",
		},
		{
			name:            "Regen Param (good value)",
			url:             "/get?url=henrygd.me&_regen_=jamesconnolly",
			expectedCode:    http.StatusOK,
			expectedBody:    "",
			expectedImage:   true,
			expectedOgCache: "MISS",
			expectedOgCode:  "1",
		},
	}

	for _, tc := range testCases {
		time.Sleep(time.Millisecond * 100)
		t.Run(tc.name, func(t *testing.T) {
			// Given
			req, err := http.NewRequest("GET", tc.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			// When
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			// Then
			assert.Equal(t, tc.expectedCode, rr.Code)

			if tc.expectedImage {
				assert.Equal(t, "image/jpeg", rr.Header().Get("Content-Type"))
				assert.Equal(t, tc.expectedOgCode, rr.Header().Get("x-og-code"))
				assert.Equal(t, tc.expectedOgCache, rr.Header().Get("x-og-cache"))
			} else {
				assert.Equal(t, tc.expectedBody, rr.Body.String())
			}
		})
	}

	// todo: more cache key tests, persistence tests, same url mutex test, CACHE_TIME test
}
