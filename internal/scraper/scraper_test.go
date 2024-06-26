package scraper_test

import (
	"strings"
	"testing"

	"github.com/henrygd/social-image-server/internal/scraper"
	"golang.org/x/net/html"
)

func TestFindOgUrl(t *testing.T) {
	tests := []struct {
		name     string
		htmlBody string
		expected string
	}{
		{
			name:     "WithOgImage",
			htmlBody: `<html><head><meta name="title" content="gotest"/><meta property="og:image" content="http://example.com/image.jpg?width=1200&cache_key=abcdef123456"></head><body><h1>hello world</h1></body></html>`,
			expected: "http://example.com/image.jpg?width=1200&cache_key=abcdef123456",
		},
		{
			name:     "WithoutOgImage",
			htmlBody: `<html><head></head></html>`,
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(test.htmlBody))
			if err != nil {
				t.Fatal("failed to parse HTML:", err)
			}

			result := scraper.FindOgUrl(doc)
			if result != test.expected {
				t.Errorf("unexpected og:image URL. Got: %s, Expected: %s", result, test.expected)
			}
		})
	}
}
