// Package config загружает конфигурацию сервиса из переменных окружения,
// флагов командной строки и JSON-файла конфигурации.
// Приоритет: переменная окружения > флаг > файл конфигурации > значение по умолчанию.
package config

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strconv"
)

// Config хранит все параметры конфигурации сервиса.
type Config struct {
	// BaseURL — адрес HTTP-сервера (например, "http://localhost:8080"); флаг -a / env SERVER_ADDRESS.
	BaseURL string
	// ResultURL — префикс для генерируемых коротких ссылок; флаг -b / env BASE_URL.
	ResultURL string
	// FileStoragePath — путь к JSON-файлу для персистентности MemStorage; флаг -f / env FILE_STORAGE_PATH.
	FileStoragePath string
	// DatabaseDSN — строка подключения к PostgreSQL; флаг -d / env DATABASE_DSN.
	// Если не задана, используется MemStorage.
	DatabaseDSN string
	// AuthSecret — секрет для подписи auth-куки HMAC-SHA256; флаг -s / env AUTH_SECRET.
	AuthSecret string
	// AuditFile — путь к файлу аудит-лога (JSON, один объект на строку); флаг -audit-file / env AUDIT_FILE.
	AuditFile string
	// AuditURL — URL удаленного приемника аудит-событий (HTTP POST); флаг -audit-url / env AUDIT_URL.
	AuditURL string
	// EnableHTTPS — включает режим HTTPS; флаг -tls / env ENABLE_HTTPS.
	EnableHTTPS bool
	// TLSCertFile — путь к PEM-файлу сертификата TLS; флаг -tls-cert / env TLS_CERT_FILE.
	TLSCertFile string
	// TLSKeyFile — путь к PEM-файлу приватного ключа TLS; флаг -tls-key / env TLS_KEY_FILE.
	TLSKeyFile string
}

// fileConfig содержит параметры конфигурации из JSON-файла.
// Все поля — указатели: nil означает «поле не задано в файле», что позволяет
// корректно реализовать приоритет флагов и переменных окружения над значениями из файла.
type fileConfig struct {
	// ServerAddress — адрес HTTP-сервера; аналог SERVER_ADDRESS / -a.
	ServerAddress *string `json:"server_address"`
	// BaseURL — префикс коротких ссылок; аналог BASE_URL / -b.
	BaseURL *string `json:"base_url"`
	// FileStoragePath — путь к файлу хранилища; аналог FILE_STORAGE_PATH / -f.
	FileStoragePath *string `json:"file_storage_path"`
	// DatabaseDSN — строка подключения к БД; аналог DATABASE_DSN / -d.
	DatabaseDSN *string `json:"database_dsn"`
	// AuthSecret — секрет подписи куки; аналог AUTH_SECRET / -s.
	AuthSecret *string `json:"auth_secret"`
	// AuditFile — путь к файлу аудит-лога; аналог AUDIT_FILE / -audit-file.
	AuditFile *string `json:"audit_file"`
	// AuditURL — URL удаленного приемника аудита; аналог AUDIT_URL / -audit-url.
	AuditURL *string `json:"audit_url"`
	// EnableHTTPS — включает HTTPS; аналог ENABLE_HTTPS / -tls.
	EnableHTTPS *bool `json:"enable_https"`
	// TLSCertFile — путь к PEM-сертификату; аналог TLS_CERT_FILE / -tls-cert.
	TLSCertFile *string `json:"tls_cert_file"`
	// TLSKeyFile — путь к PEM-ключу; аналог TLS_KEY_FILE / -tls-key.
	TLSKeyFile *string `json:"tls_key_file"`
}

// loadFileConfig читает и разбирает JSON-файл конфигурации по заданному пути.
// Возвращает nil, nil если путь пустой.
// При ошибке чтения или разбора файла возвращает ошибку.
func loadFileConfig(path string) (*fileConfig, error) {
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var fc fileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, err
	}

	return &fc, nil
}

// resolveString возвращает первое непустое значение в порядке приоритета:
// env > flagVal > fileVal > defaultVal.
func resolveString(env, flagVal string, fileVal *string, defaultVal string) string {
	if env != "" {
		return env
	}
	if flagVal != "" {
		return flagVal
	}
	if fileVal != nil && *fileVal != "" {
		return *fileVal
	}
	return defaultVal
}

// resolveBool возвращает значение булевого параметра с учетом приоритетов:
// env (непустой) > flagExplicit (флаг явно передан пользователем) > fileVal > defaultVal.
func resolveBool(env string, flagVal bool, flagExplicit bool, fileVal *bool, defaultVal bool) bool {
	if env != "" {
		v, _ := strconv.ParseBool(env)
		return v
	}
	if flagExplicit {
		return flagVal
	}
	if fileVal != nil {
		return *fileVal
	}
	return defaultVal
}

// LoadConfig читает конфигурацию из JSON-файла, переменных окружения и флагов командной строки.
// Путь к JSON-файлу задается флагом -c/-config или переменной окружения CONFIG.
// Должна вызываться один раз при старте приложения.
func LoadConfig() *Config {
	defaultURL := "http://localhost:8080"
	defaultFileStorage := "shortener_data.json"

	// Имена флагов командной строки
	const (
		flagNameConfigFile  = "c"
		flagNameConfigFile2 = "config"
		flagNameServerURL   = "a"
		flagNameBaseURL     = "b"
		flagNameFileStorage = "f"
		flagNameDatabaseDSN = "d"
		flagNameAuthSecret  = "s"
		flagNameAuditFile   = "audit-file"
		flagNameAuditURL    = "audit-url"
		flagNameEnableHTTPS = "tls"
		flagNameTLSCert     = "tls-cert"
		flagNameTLSKey      = "tls-key"
	)

	// Имена переменных окружения
	const (
		envNameConfig      = "CONFIG"
		envNameServerURL   = "SERVER_ADDRESS"
		envNameBaseURL     = "BASE_URL"
		envNameFileStorage = "FILE_STORAGE_PATH"
		envNameDatabaseDSN = "DATABASE_DSN"
		envNameAuthSecret  = "AUTH_SECRET"
		envNameAuditFile   = "AUDIT_FILE"
		envNameAuditURL    = "AUDIT_URL"
		envNameEnableHTTPS = "ENABLE_HTTPS"
		envNameTLSCert     = "TLS_CERT_FILE"
		envNameTLSKey      = "TLS_KEY_FILE"
	)

	// Определяем флаги командной строки
	flagConfigFile := flag.String(flagNameConfigFile, "", "path to JSON config file")
	flagConfigFile2 := flag.String(flagNameConfigFile2, "", "path to JSON config file (long form)")
	flagServerURL := flag.String(flagNameServerURL, "", "server base url")
	flagResultURL := flag.String(flagNameBaseURL, "", "server result url")
	flagFileStorage := flag.String(flagNameFileStorage, "", "file storage path")
	flagDatabaseDSN := flag.String(flagNameDatabaseDSN, "", "database connection DSN")
	flagAuthSecret := flag.String(flagNameAuthSecret, "", "auth secret for cookies")
	flagAuditFile := flag.String(flagNameAuditFile, "", "path to audit log file")
	flagAuditURL := flag.String(flagNameAuditURL, "", "URL of remote audit receiver")
	flagEnableHTTPS := flag.Bool(flagNameEnableHTTPS, false, "enable HTTPS mode")
	flagTLSCert := flag.String(flagNameTLSCert, "", "path to TLS certificate PEM file")
	flagTLSKey := flag.String(flagNameTLSKey, "", "path to TLS private key PEM file")
	flag.Parse()

	// Определяем, какие булевые флаги были явно переданы пользователем
	explicitFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = true
	})

	// Определяем путь к файлу конфигурации: env CONFIG > флаг -c/-config
	configPath := os.Getenv(envNameConfig)
	if configPath == "" {
		configPath = *flagConfigFile
	}
	if configPath == "" {
		configPath = *flagConfigFile2
	}

	// Загружаем файл конфигурации (если путь задан)
	fc, err := loadFileConfig(configPath)
	if err != nil {
		log.Fatalf("failed to load config file %q: %v", configPath, err)
	}

	// Читаем переменные окружения
	envServerURL := os.Getenv(envNameServerURL)
	envResultURL := os.Getenv(envNameBaseURL)
	envFileStorage := os.Getenv(envNameFileStorage)
	envDatabaseDSN := os.Getenv(envNameDatabaseDSN)
	envAuthSecret := os.Getenv(envNameAuthSecret)
	envAuditFile := os.Getenv(envNameAuditFile)
	envAuditURL := os.Getenv(envNameAuditURL)
	envEnableHTTPS := os.Getenv(envNameEnableHTTPS)
	envTLSCert := os.Getenv(envNameTLSCert)
	envTLSKey := os.Getenv(envNameTLSKey)

	// Если файл конфигурации не задан, используем пустую структуру для упрощения кода ниже
	if fc == nil {
		fc = &fileConfig{}
	}

	return &Config{
		BaseURL:         resolveString(envServerURL, *flagServerURL, fc.ServerAddress, defaultURL),
		ResultURL:       resolveString(envResultURL, *flagResultURL, fc.BaseURL, defaultURL),
		FileStoragePath: resolveString(envFileStorage, *flagFileStorage, fc.FileStoragePath, defaultFileStorage),
		DatabaseDSN:     resolveString(envDatabaseDSN, *flagDatabaseDSN, fc.DatabaseDSN, ""),
		AuthSecret:      resolveString(envAuthSecret, *flagAuthSecret, fc.AuthSecret, "dev_secret"),
		AuditFile:       resolveString(envAuditFile, *flagAuditFile, fc.AuditFile, ""),
		AuditURL:        resolveString(envAuditURL, *flagAuditURL, fc.AuditURL, ""),
		EnableHTTPS:     resolveBool(envEnableHTTPS, *flagEnableHTTPS, explicitFlags[flagNameEnableHTTPS], fc.EnableHTTPS, false),
		TLSCertFile:     resolveString(envTLSCert, *flagTLSCert, fc.TLSCertFile, ""),
		TLSKeyFile:      resolveString(envTLSKey, *flagTLSKey, fc.TLSKeyFile, ""),
	}
}
