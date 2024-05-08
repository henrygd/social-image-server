package scraper

import (
	"net/url"

	"golang.org/x/net/html"
)

var Parse = html.Parse

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

// extract the cache_key parameter from a URL
func ExtractCacheKey(ogImageURL string) (string, error) {
	ogImageURLParsed, err := url.Parse(ogImageURL)
	if err != nil {
		return "", err
	}
	cacheKey := ogImageURLParsed.Query().Get("cache_key")
	return cacheKey, nil
}
