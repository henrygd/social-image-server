package scraper

import (
	"net/http"
	"time"

	"golang.org/x/net/html"
)

var client *http.Client

// Returns http.Client with 10 second timeout
func GetClient() *http.Client {
	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Second,
		}
	}
	return client
}

// find og:image meta tag and extract the content attribute
func FindOgUrl(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "meta" {
		for _, attr := range n.Attr {
			if attr.Key == "property" && attr.Val == "og:image" {
				// found it. now extract the content attribute.
				for _, subAttr := range n.Attr {
					if subAttr.Key == "content" {
						return subAttr.Val
					}
				}
			}
		}
	}
	// recursively search for the meta tag in child nodes
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := FindOgUrl(c); result != "" {
			return result
		}
	}
	return ""
}
