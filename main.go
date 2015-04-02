package main

import (
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/golang/groupcache"
	"github.com/namsral/flag"
)

var (
	// Enable config functionality
	configFlag = flag.String("config", "", "Config file to read")
	// Log output flags
	logFormatter   = flag.String("log_formatter", "text", "Log formatter type")
	logForceColors = flag.Bool("log_force_colors", false, "Force colored prompt?")
	// Proxy server settings
	proxyBind = flag.String("proxy_bind", ":5000", "Bind address of the proxy server")
	// Groupcache settings
	cacheBind   = flag.String("cache_bind", ":5001", "Bind address of the groupcache server")
	cachePublic = flag.String("cache_public", "", "Public address of the groupcache server")
	cachePeers  = flag.String("cache_peers", "", "List of peers in the groupcache cluster")
	cacheSize   = flag.Int64("cache_size", 64<<20, "Size of the LRU cache")
)

var (
	ErrInvalidContentType = errors.New("Invalid Content-Type")
)

func main() {
	// Parse the flags
	flag.Parse()

	// Create a new groupcache pool
	pool := groupcache.NewHTTPPool(*cachePublic)
	pool.Set(strings.Split(*cachePeers, ",")...)

	// Listen and serve the groupcache pool
	cacheServer := http.Server{
		Addr:    *cacheBind,
		Handler: pool,
	}
	go func() {
		log.Printf("Starting up the cache HTTP server on address %s", *cacheBind)

		err := cacheServer.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()

	// Create a new groupcache pool
	cache := groupcache.NewGroup("ritratt", *cacheSize, groupcache.GetterFunc(func(ctx groupcache.Context, url string, dest groupcache.Sink) error {
		// First try with https
		schema := "https://"
		resp, err := http.Head("https://" + url)
		if err != nil {
			log.Printf("[https] Error while querying %s: %s", url, err)

			// https doesn't work, try http
			schema = "http://"
			resp, err = http.Head("http://" + url)
			if err != nil {
				log.Printf("[http] Error while querying %s: %s", url, err)
			}
		}

		// Content-Type of the result has to start with image/
		// We also don't support SVGs, check out this link for more information:
		// https://www.owasp.org/images/0/03/Mario_Heiderich_OWASP_Sweden_The_image_that_called_me.pdf
		ct := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "image/") || strings.Contains(ct, "image/svg+xml") {
			log.Printf("[head] Invalid Content-Type of %s", url)
			return ErrInvalidContentType
		}

		// Query the proper URL, now including the body
		resp, err = http.Get(schema + url)
		defer resp.Body.Close()
		if err != nil {
			log.Printf("[get] Error while querying %s: %s", url, err)
		} else {
			log.Printf("[get] Loaded %s", url)
		}

		// Content-Type check #2
		ct = resp.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "image/") || strings.Contains(ct, "image/svg+xml") {
			log.Printf("[get] Invalid Content-Type of %s", url)
			return ErrInvalidContentType
		}

		// Read the body
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		// Put the body into cache with the Content-Type
		dest.SetString(ct + ";" + string(body))

		return nil
	}))

	log.Printf("Starting up the proxy HTTP server on address %s", *proxyBind)
	proxyServer := http.Server{
		Addr: *proxyBind,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Index page
			if len(r.RequestURI) < 3 || r.RequestURI[:3] != "/i/" {
				w.Write([]byte("lavab/ritratt"))
				return
			}

			// Get the data from groupcache
			var data string
			err := cache.Get(nil, r.RequestURI[3:], groupcache.StringSink(&data))
			if err != nil {
				w.Write([]byte(err.Error()))
				return
			}

			// Split the result into two parts
			parts := strings.SplitN(data, ";", 2)

			// Set the content type
			w.Header().Set("Content-Type", parts[0])

			// Write the body
			w.Write([]byte(parts[1]))
		}),
	}
	log.Fatal(proxyServer.ListenAndServe())
}
