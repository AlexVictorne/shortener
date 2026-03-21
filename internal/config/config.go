package config

import (
	"flag"
	"os"
)

type Config struct {
	BaseURL   string
	ResultURL string
}

func LoadConfig() *Config {
	defaultURL := "http://localhost:8080"

	flagNameServerURL := "a"
	envNameServerURL := "SERVER_ADDRESS"
	flagNameBaseURL := "b"
	envNameBaseURL := "BASE_URL"

	var serverURL string
	var resultURL string

	envServerURL := os.Getenv(envNameServerURL)
	envResultURL := os.Getenv(envNameBaseURL)

	flagServerURL := flag.String(flagNameServerURL, "", "server base url")
	flagResultURL := flag.String(flagNameBaseURL, "", "server result url")
	flag.Parse()

	// Приоритет env > flag > default
	if envServerURL != "" {
		serverURL = envServerURL
	} else if *flagServerURL != "" {
		serverURL = *flagServerURL
	} else {
		serverURL = defaultURL
	}

	if envResultURL != "" {
		resultURL = envResultURL
	} else if *flagResultURL != "" {
		resultURL = *flagResultURL
	} else {
		resultURL = defaultURL
	}

	return &Config{
		BaseURL:   serverURL,
		ResultURL: resultURL,
	}
}
