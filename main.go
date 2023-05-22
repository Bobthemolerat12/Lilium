package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"io/ioutil"
	"net/url"
	"strings"
	"path"
	"time"
	"bytes"
	"regexp"
	"github.com/DaRealFreak/cloudflare-bp-go"
	"github.com/PuerkitoBio/goquery"
)

func main() {
	http.HandleFunc("/fetch", fetchHandler)
	http.HandleFunc("/proxy", proxyHandler)
	http.HandleFunc("/", staticHandler)
	log.Println("Server started on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func staticHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method == "GET" {
        if r.URL.Path == "/style.css" {
            cssBytes, err := ioutil.ReadFile("static/style.css")
            if err != nil {
                http.Error(w, "Failed to read style.css file", http.StatusInternalServerError)
                return
            }
            w.Header().Set("Content-Type", "text/css; charset=utf-8")
            w.Write(cssBytes)
            return
        }
        if r.URL.Path == "/bg.png" {
            pngBytes, err := ioutil.ReadFile("static/bg.png")
            if err != nil {
                http.Error(w, "Failed to read bg.png file", http.StatusInternalServerError)
                return
            }
            w.Header().Set("Content-Type", "image/png")
            w.Write(pngBytes)
            return
        }
		if r.URL.Path == "/lilium-icon-borders.ico" {
            pngBytes, err := ioutil.ReadFile("static/lilium-icon-borders.ico")
            if err != nil {
                http.Error(w, "Failed to read lilium-icon-borders.ico file", http.StatusInternalServerError)
                return
            }
            w.Header().Set("Content-Type", "image/png")
            w.Write(pngBytes)
            return
        }
        if r.URL.Path == "/" || r.URL.Path == "/index.html" {
            htmlBytes, err := ioutil.ReadFile("static/index.html")
            if err != nil {
                http.Error(w, "Failed to read index.html file", http.StatusInternalServerError)
                return
            }

            w.Header().Set("Content-Type", "text/html; charset=utf-8")
            w.Write(htmlBytes)
            return
        }
      w.Header().Set("Content-Type", "text/html; charset=utf-8")
      w.WriteHeader(http.StatusNotFound)
      four04Bytes, err := ioutil.ReadFile("static/404.html")
      if err != nil {
          http.Error(w, "Failed to read 404.html file", http.StatusInternalServerError)
          return
      }
      w.Write(four04Bytes)
    } else {
        http.Redirect(w, r, "/fetch?url="+r.URL.Query().Get("url"), http.StatusSeeOther)
    }
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
 	w.Header().Set("Origin", urlParam)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "*")
	w.Header().Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36")
	fmt.Fprint(w, modifiedHTML)
}


func fetchHTML(urlParam string) (string, error) {
	req, err := http.NewRequest("GET", urlParam, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{}
	client.Transport = cloudflarebp.AddCloudFlareByPass(client.Transport)
  	req.Header.Set("Origin", urlParam)
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
	
	html, err = replaceScriptTags(html, urlParam)
	if err != nil {
		fmt.Println("Error:", err)
	}
	return html, nil
}

func replaceScriptTags(html string, currentURL string) (string, error) {
	// Load the HTML document using goquery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}

	// Find all script tags and modify their src attribute
	doc.Find("html").Each(func(i int, s *goquery.Selection) {
		if src, exists := s.Attr("src"); exists {
			updatedSrc := fmt.Sprintf("%s%s", currentURL, src)
			s.SetAttr("src", updatedSrc)
		}
		if src, exists := s.Attr("href"); exists {
			updatedSrc := fmt.Sprintf("%s%s", currentURL, src)
			s.SetAttr("href", updatedSrc)
		}
	})

	// Get the modified HTML content
	modifiedHTML, err := doc.Html()
	if err != nil {
		return "", err
	}

	return modifiedHTML, nil
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
			} else if s.Is("link") && s.AttrOr("as", "") == "script" {
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
              fetchURL := fmt.Sprintf("%s/fetch?url=%s", currentURL, url.QueryEscape(modifiedURL))
	            s.SetAttr("src", fetchURL)
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
	proto := "https"
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
		urlParam = "https://" + urlParam
	}

	parsedURL, err := url.Parse(urlParam)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	client := &http.Client{}
	client.Transport = cloudflarebp.AddCloudFlareByPass(client.Transport)
	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36")
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "*")
  	req.Header.Set("Origin", urlParam)

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



