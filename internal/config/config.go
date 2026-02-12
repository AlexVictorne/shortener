package config

import (
	"flag"
	"log"
	"net/url"
	"strings"
)

type Config struct {
	BaseURL   string
	ResultURL string
}

func LoadConfig() *Config {
	defaultURL := "http://localhost:8080"

	var (
		baseURL   = flag.String("a", defaultURL, "server base url")
		resultURL = flag.String("b", defaultURL, "server result url")
	)
	flag.Parse()

	validatedBaseURL := ValidateURL(*baseURL, defaultURL, "BaseURL")
	validatedResultURL := ValidateURL(*resultURL, defaultURL, "ResultURL")

	return &Config{
		BaseURL:   validatedBaseURL,
		ResultURL: validatedResultURL,
	}
}

func ValidateURL(val string, def string, name string) string {
	if val == "" {
		log.Printf("%s is empty, using default: %s", name, def)
		return def
	}

	origVal := val
	// Если схема отсутствует, добавляем http
	if !strings.Contains(val, "http") {
		log.Printf("%s: unsupported scheme, using default scheme: http", name)
		val = "http://" + val
	}

	u, err := url.Parse(val)

	if err != nil {
		log.Printf("%s is invalid ('%s'), using default: %s", name, origVal, def)
		return def
	}

	scheme := u.Scheme
	hostname := u.Hostname()
	port := u.Port()

	defURL, err := url.Parse(def)
	if err != nil {
		log.Printf("%s is invalid ('%s'), but we using it", "defURL", def)
		return def
	}

	defScheme := defURL.Scheme
	defHostname := defURL.Hostname()
	defPort := defURL.Port()

	if hostname == "" {
		log.Printf("%s: host is empty or invalid ('%s'), using default host: %s", name, hostname, defHostname)
		hostname = defHostname
	}

	if port == "" {
		log.Printf("%s: port is missing, using default port: %s", name, defPort)
		port = defPort
	}

	if scheme == "" {
		log.Printf("%s: scheme is missing, using default scheme: %s", name, scheme)
		scheme = defScheme
	}

	u, err = url.Parse(scheme + "://" + hostname + ":" + port)
	if err != nil {
		log.Printf("%s is invalid ('%s'), using default: %s", "Correct URL", origVal, def)
		return def
	}

	return u.String()
}
