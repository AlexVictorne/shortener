package config

import "os"

type Config struct {
	Host    string
	Port    string
	BaseURL string
}

func LoadConfig() *Config {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	host := os.Getenv("SERVER_HOST")
	if host == "" {
		host = "localhost"
	}

	protocol := os.Getenv("SERVER_PROTOCOL")
	if protocol == "" {
		protocol = "http"
	}

	baseURL := protocol + "://" + host + ":" + port

	return &Config{
		Host:    host,
		Port:    port,
		BaseURL: baseURL,
	}
}
