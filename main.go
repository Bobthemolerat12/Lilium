package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"path"
	"time"
	"bytes"
	"regexp"
	"github.com/PuerkitoBio/goquery"
)

func main() {
	http.HandleFunc("/fetch", fetchHandler)
	http.HandleFunc("/proxy", proxyHandler)
	fs := http.FileServer(http.Dir("static"))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	    http.ServeFile(w, r, "static/index.html")
	})
	http.Handle("/static/", http.StripPrefix("/static/", fs))
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

	// Check if the fetched URL is a file
	ext := path.Ext(urlParam)
	if ext == ".js" || ext == ".css" {
		// Remove HTML tags from the file contents
		re := regexp.MustCompile("<[^>]*>")
		plainText := re.ReplaceAllString(fetchedHTML, "")

		// Send the plain text as raw text
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader([]byte(plainText)))
		return
	}

	// Modify HTML links
	modifiedHTML := modifyHTMLLinks(fetchedHTML, currentURL, urlParam)

	// Send the modified HTML
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "*")
	w.Header().Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36")
	fmt.Fprint(w, modifiedHTML)
}

func mainPage(w http.ResponseWriter, r *http.Request) {
    // Create a file server that serves static files from the "static" directory
    fs := http.FileServer(http.Dir("static"))

    // Serve the "index.html" file located in the "static" directory
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, "static/index.html")
    })

    // Serve all other static files from the file server
    http.Handle("/static/", http.StripPrefix("/static/", fs))
}

func fetchHTML(urlParam string) (string, error) {
	req, err := http.NewRequest("GET", urlParam, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{}
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	contentType := resp.Header.Get("Content-Type")
	req.Header.Set("Content-Type", contentType)
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

	doc.Find("a[href], link[href], script[src], img[src], meta[content]").Each(func(i int, s *goquery.Selection) {
	    if attr, exists := s.Attr("href"); exists {
	        if s.Is("link") && (s.AttrOr("rel", "") == "icon" || s.AttrOr("rel", "") == "shortcut icon") {
	            // Handle favicon links
	            modifiedURL := modifyURL(attr)
	            if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
	                modifiedURL = currentScheme + "://" + modifiedURL
	            }
	            proxyURL := fmt.Sprintf("%s/proxy?url=%s", currentURL, url.QueryEscape(modifiedURL))
	            s.SetAttr("href", proxyURL)
	        } else if s.Is("link") && s.AttrOr("rel", "") == "stylesheet" {
	            // Handle stylesheet links
	            modifiedURL := modifyURL(attr)
	            if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
	                modifiedURL = currentScheme + "://" + modifiedURL
	            }
	            proxyURL := fmt.Sprintf("%s/proxy?url=%s", currentURL, url.QueryEscape(modifiedURL))
	            s.SetAttr("href", proxyURL)
	        } else if s.Is("link") && shouldProxyImage(attr) {
	            // Handle link images
	            modifiedURL := modifyURL(attr)
	            if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
	                modifiedURL = currentScheme + "://" + modifiedURL
	            }
	            proxyURL := fmt.Sprintf("%s/proxy?url=%s", currentURL, url.QueryEscape(modifiedURL))
	            s.SetAttr("href", proxyURL)
	        } else if s.Is("link") {
	            // Handle link images
	            modifiedURL := modifyURL(attr)
	            if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
	                modifiedURL = currentScheme + "://" + modifiedURL
	            }
	            fetchURL := fmt.Sprintf("%s/fetch?url=%s", currentURL, url.QueryEscape(modifiedURL))
	            s.SetAttr("href", fetchURL)
	        } else {
	            // Handle other links
	            modifiedURL := modifyURL(attr)
	            if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
	                modifiedURL = currentScheme + "://" + modifiedURL
	            }
	            fetchURL := fmt.Sprintf("%s/fetch?url=%s", currentURL, url.QueryEscape(modifiedURL))
	            s.SetAttr("href", fetchURL)
	        }
	    }

	    if attr, exists := s.Attr("meta"); exists {
	    	if s.Is("meta") {
	    	    // Handle meta images
	    	    modifiedURL := modifyURL(attr)
	    	    if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
	    	        modifiedURL = currentScheme + "://" + modifiedURL
	    	    }
	    	    fetchURL := fmt.Sprintf("%s/proxy?url=%s", currentURL, url.QueryEscape(modifiedURL))
	    	    s.SetAttr("content", fetchURL)
	        } else {
	            // Handle other meta links
	            modifiedURL := modifyURL(attr)
	            if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
	                modifiedURL = currentScheme + "://" + modifiedURL
	            }
	            fetchURL := fmt.Sprintf("%s/fetch?url=%s", currentURL, url.QueryEscape(modifiedURL))
	            s.SetAttr("href", fetchURL)
	        }
	    }

	    if attr, exists := s.Attr("src"); exists {
	        if s.Is("script") && (strings.Contains(attr, "js") || strings.Contains(attr, ".js")) {
	            // Handle script tags with "js" in the src attribute
	            modifiedURL := modifyURL(attr)
	            if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
	                modifiedURL = currentScheme + "://" + modifiedURL
	            }
	            fetchURL := fmt.Sprintf("%s/proxy?url=%s", currentURL, url.QueryEscape(modifiedURL))
	            s.SetAttr("src", fetchURL)
	        } else if s.Is("script") {
	            // Handle other script tags
	            modifiedURL := modifyURL(attr)
	            if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
	                modifiedURL = currentScheme + "://" + modifiedURL
	            }
	            fetchURL := fmt.Sprintf("%s/proxy?url=%s", currentURL, url.QueryEscape(modifiedURL))
	            s.SetAttr("src", fetchURL)
	        } else if shouldProxyImage(attr) {
	            modifiedURL := modifyURL(attr)
	            if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
	                modifiedURL = currentScheme + "://" + modifiedURL
	            }
	            proxyURL := fmt.Sprintf("%s/proxy?url=%s", currentURL, url.QueryEscape(modifiedURL))
	            s.SetAttr("src", proxyURL)
		    } else if s.Is("script") && s.AttrOr("async", "") != "" {
		        // Handle script tags with "async" attribute
		        modifiedURL := modifyURL(attr)
		        if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
		            modifiedURL = currentScheme + "://" + modifiedURL
		        }
		        fetchURL := fmt.Sprintf("%s/proxy?url=%s", currentURL, url.QueryEscape(modifiedURL))
		        s.SetAttr("src", fetchURL)
		    } else {
	            modifiedURL := modifyURL(attr)
	            if !strings.HasPrefix(modifiedURL, "http://") && !strings.HasPrefix(modifiedURL, "https://") {
	                modifiedURL = currentScheme + "://" + modifiedURL
	            }
	            s.SetAttr("src", modifiedURL)
	        }
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
	imageExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".ico", ".svg"}

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
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "*")

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



