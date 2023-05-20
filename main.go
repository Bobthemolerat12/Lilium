package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func main() {
	http.HandleFunc("/fetch", fetchHandler)
	http.HandleFunc("/proxy", proxyHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func fetchHandler(w http.ResponseWriter, r *http.Request) {
	urlParam := r.URL.Query().Get("url")
	if urlParam == "" {
		http.Error(w, "Missing 'url' parameter", http.StatusBadRequest)
		return
	}

	fetchedHTML, err := fetchHTML(urlParam)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	currentURL := getCurrentURL(r)
	modifiedHTML := modifyHTMLLinks(fetchedHTML, currentURL, urlParam)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, modifiedHTML)
}

func fetchHTML(urlParam string) (string, error) {
	req, err := http.NewRequest("GET", urlParam, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	html, err := doc.Html()
	if err != nil {
		return "", err
	}

	return html, nil
}

func modifyHTMLLinks(html, currentURL, originalURL string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Println("Failed to parse HTML:", err)
		return html
	}

	baseURL, err := url.Parse(originalURL)
	if err != nil {
		log.Println("Failed to parse original URL:", err)
		return html
	}

	currentScheme := baseURL.Scheme

	modifyURL := func(link string) string {
		parsedURL, err := url.Parse(link)
		if err != nil {
			log.Println("Failed to parse URL:", err)
			return link
		}

		// Check if the parsed URL is absolute
		if parsedURL.IsAbs() {
			// Convert HTTP URLs to HTTPS
			if parsedURL.Scheme == "http" {
				parsedURL.Scheme = "https"
			}
			return parsedURL.String()
		}

		// Relative URL, resolve it against the base URL
		resolvedURL := baseURL.ResolveReference(parsedURL)
		return resolvedURL.String()
	}

	doc.Find("a[href], link[href], script[src], img[src]").Each(func(i int, s *goquery.Selection) {
		if attr, exists := s.Attr("href"); exists {
			s.SetAttr("href", modifyURL(attr))
		}

		if attr, exists := s.Attr("src"); exists {
			if shouldProxyImage(attr) {
				proxyURL := fmt.Sprintf("%s/proxy?url=%s", currentURL, url.QueryEscape(attr))
				s.SetAttr("src", proxyURL)
			} else {
				modifiedURL := modifyURL(attr)
				if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
					modifiedURL = currentScheme + "://" + modifiedURL
				}
				s.SetAttr("src", modifiedURL)
			}
			s.SetAttr("loading", "lazy") // Add 'loading="lazy"' attribute for lazy loading
		}
	})

	modifiedHTML, err := doc.Html()
	if err != nil {
		log.Println("Failed to generate modified HTML:", err)
		return html
	}

	return modifiedHTML
}


func shouldProxyImage(url string) bool {
	imageExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp"}

	for _, ext := range imageExtensions {
		if strings.HasSuffix(strings.ToLower(url), ext) {
			return true
		}
	}

	return false
}


func modifyURL(link, currentURL, originalURL string) string {
	parsedURL, err := url.Parse(link)
	if err != nil {
		log.Println("Failed to parse URL:", err)
		return link
	}

	parsedOriginalURL, err := url.Parse(originalURL)
	if err != nil {
		log.Println("Failed to parse original URL:", err)
		return link
	}

	if parsedURL.IsAbs() {
		return fmt.Sprintf("%s/fetch?url=%s", currentURL, parsedURL.String())
	}

	modifiedURL := parsedOriginalURL.ResolveReference(parsedURL)
	if modifiedURL.Scheme == "" {
		modifiedURL.Scheme = parsedOriginalURL.Scheme
	}

	proxyURL := fmt.Sprintf("%s/proxy?url=%s", currentURL, url.QueryEscape(modifiedURL.String()))
	return proxyURL
}



func getCurrentURL(r *http.Request) string {
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}

	host := r.Host
	return fmt.Sprintf("%s://%s", proto, host)
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	urlParam := r.URL.Query().Get("url")
	if urlParam == "" {
		http.Error(w, "Missing 'url' parameter", http.StatusBadRequest)
		return
	}

	// Add support for both http:// and https:// schemes
	if !strings.HasPrefix(urlParam, "http://") && !strings.HasPrefix(urlParam, "https://") {
		urlParam = "http://" + urlParam
	}

	parsedURL, err := url.Parse(urlParam)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	w.Header().Set("Content-Type", contentType)

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Println("Failed to copy response:", err)
		return
	}
}



