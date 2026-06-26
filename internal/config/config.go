package config

import (
	"flag"
	"os"
)

type Config struct {
	BaseURL         string
	ResultURL       string
	FileStoragePath string
	DatabaseDSN     string
	AuthSecret      string
	AuditFile       string
	AuditURL        string
}

func LoadConfig() *Config {
	defaultURL := "http://localhost:8080"
	defaultFileStorage := "shortener_data.json"

	flagNameServerURL := "a"
	envNameServerURL := "SERVER_ADDRESS"
	flagNameBaseURL := "b"
	envNameBaseURL := "BASE_URL"
	flagNameFileStorage := "f"
	envNameFileStorage := "FILE_STORAGE_PATH"
	flagNameDatabaseDSN := "d"
	envNameDatabaseDSN := "DATABASE_DSN"
	flagNameAuthSecret := "s"
	envNameAuthSecret := "AUTH_SECRET"
	flagNameAuditFile := "audit-file"
	envNameAuditFile := "AUDIT_FILE"
	flagNameAuditURL := "audit-url"
	envNameAuditURL := "AUDIT_URL"

	var serverURL string
	var resultURL string
	var fileStoragePath string
	var databaseDSN string
	var authSecret string
	var auditFile string
	var auditURL string

	envServerURL := os.Getenv(envNameServerURL)
	envResultURL := os.Getenv(envNameBaseURL)
	envFileStorage := os.Getenv(envNameFileStorage)
	envDatabaseDSN := os.Getenv(envNameDatabaseDSN)
	envAuthSecret := os.Getenv(envNameAuthSecret)
	envAuditFile := os.Getenv(envNameAuditFile)
	envAuditURL := os.Getenv(envNameAuditURL)

	flagServerURL := flag.String(flagNameServerURL, "", "server base url")
	flagResultURL := flag.String(flagNameBaseURL, "", "server result url")
	flagFileStorage := flag.String(flagNameFileStorage, "", "file storage path")
	flagDatabaseDSN := flag.String(flagNameDatabaseDSN, "", "database connection DSN")
	flagAuthSecret := flag.String(flagNameAuthSecret, "", "auth secret for cookies")
	flagAuditFile := flag.String(flagNameAuditFile, "", "path to audit log file")
	flagAuditURL := flag.String(flagNameAuditURL, "", "URL of remote audit receiver")
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

	if envFileStorage != "" {
		fileStoragePath = envFileStorage
	} else if *flagFileStorage != "" {
		fileStoragePath = *flagFileStorage
	} else {
		fileStoragePath = defaultFileStorage
	}

	if envDatabaseDSN != "" {
		databaseDSN = envDatabaseDSN
	} else if *flagDatabaseDSN != "" {
		databaseDSN = *flagDatabaseDSN
	}

	if envAuthSecret != "" {
		authSecret = envAuthSecret
	} else if *flagAuthSecret != "" {
		authSecret = *flagAuthSecret
	} else {
		authSecret = "dev_secret"
	}

	if envAuditFile != "" {
		auditFile = envAuditFile
	} else if *flagAuditFile != "" {
		auditFile = *flagAuditFile
	}

	if envAuditURL != "" {
		auditURL = envAuditURL
	} else if *flagAuditURL != "" {
		auditURL = *flagAuditURL
	}

	return &Config{
		BaseURL:         serverURL,
		ResultURL:       resultURL,
		FileStoragePath: fileStoragePath,
		DatabaseDSN:     databaseDSN,
		AuthSecret:      authSecret,
		AuditFile:       auditFile,
		AuditURL:        auditURL,
	}
}
