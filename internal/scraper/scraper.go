package scraper

import (
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
