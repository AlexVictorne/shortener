package config

import (
	"flag"
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

	return &Config{
		BaseURL:   *baseURL,
		ResultURL: *resultURL,
	}
}
