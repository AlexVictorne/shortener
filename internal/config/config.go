// Package config загружает конфигурацию сервиса из переменных окружения,
// флагов командной строки и JSON-файла конфигурации с помощью github.com/spf13/viper.
// Приоритет: переменная окружения > флаг > файл конфигурации > значение по умолчанию.
//
// Флаги парсятся стандартным пакетом "flag", а не spf13/pflag: pflag разбирает
// многобуквенные флаги вида -tls-cert или -config некорректно при вызове через одиночный дефис.
// Поэтому viper здесь используется только для слоя файл/default, а явно переданные флаги применяются
// поверх через viper.Set — но только если соответствующая переменная окружения
// не задана, иначе итоговое значение и так возьмётся из env-слоя viper (BindEnv).
package config

import (
	"flag"
	"log"
	"os"

	"github.com/spf13/viper"
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
	// AuthSecret — секрет для подписи auth-куки HMAC-SHA256; флаг -auth-secret / env AUTH_SECRET.
	AuthSecret string
	// AuditFile — путь к файлу аудит-лога (JSON, один объект на строку); флаг -audit-file / env AUDIT_FILE.
	AuditFile string
	// AuditURL — URL удаленного приемника аудит-событий (HTTP POST); флаг -audit-url / env AUDIT_URL.
	AuditURL string
	// EnableHTTPS — включает режим HTTPS; флаг -s / env ENABLE_HTTPS.
	EnableHTTPS bool
	// TLSCertFile — путь к PEM-файлу сертификата TLS; флаг -tls-cert / env TLS_CERT_FILE.
	TLSCertFile string
	// TLSKeyFile — путь к PEM-файлу приватного ключа TLS; флаг -tls-key / env TLS_KEY_FILE.
	TLSKeyFile string
}

const (
	flagNameConfigFile  = "c"
	flagNameConfigFile2 = "config"
	envNameConfig       = "CONFIG"
)

// LoadConfig читает конфигурацию из JSON-файла, переменных окружения и флагов командной строки.
// Путь к JSON-файлу задается флагом -c/-config или переменной окружения CONFIG.
// Приоритет источников: переменная окружения > флаг > файл конфигурации > значение по умолчанию.
// Должна вызываться один раз при старте приложения.
func LoadConfig() *Config {
	defaultURL := "http://localhost:8080"
	defaultFileStorage := "shortener_data.json"

	flagConfigFile := flag.String(flagNameConfigFile, "", "path to JSON config file")
	flagConfigFile2 := flag.String(flagNameConfigFile2, "", "path to JSON config file (long form)")
	flagServerURL := flag.String("a", "", "server base url")
	flagResultURL := flag.String("b", "", "server result url")
	flagFileStorage := flag.String("f", "", "file storage path")
	flagDatabaseDSN := flag.String("d", "", "database connection DSN")
	flagAuthSecret := flag.String("auth-secret", "", "auth secret for cookies")
	flagAuditFile := flag.String("audit-file", "", "path to audit log file")
	flagAuditURL := flag.String("audit-url", "", "URL of remote audit receiver")
	flagEnableHTTPS := flag.Bool("s", false, "enable HTTPS mode")
	flagTLSCert := flag.String("tls-cert", "", "path to TLS certificate PEM file")
	flagTLSKey := flag.String("tls-key", "", "path to TLS private key PEM file")
	flag.Parse()

	v := viper.New()
	v.SetDefault("server_address", defaultURL)
	v.SetDefault("base_url", defaultURL)
	v.SetDefault("file_storage_path", defaultFileStorage)
	v.SetDefault("auth_secret", "dev_secret")

	for key, envName := range map[string]string{
		"server_address":    "SERVER_ADDRESS",
		"base_url":          "BASE_URL",
		"file_storage_path": "FILE_STORAGE_PATH",
		"database_dsn":      "DATABASE_DSN",
		"auth_secret":       "AUTH_SECRET",
		"audit_file":        "AUDIT_FILE",
		"audit_url":         "AUDIT_URL",
		"enable_https":      "ENABLE_HTTPS",
		"tls_cert_file":     "TLS_CERT_FILE",
		"tls_key_file":      "TLS_KEY_FILE",
	} {
		if err := v.BindEnv(key, envName); err != nil {
			log.Fatalf("failed to bind env %q: %v", envName, err)
		}
	}

	// Путь к файлу конфигурации: env CONFIG > флаг -c/-config
	configPath := os.Getenv(envNameConfig)
	if configPath == "" {
		configPath = *flagConfigFile
	}
	if configPath == "" {
		configPath = *flagConfigFile2
	}

	if configPath != "" {
		v.SetConfigFile(configPath)
		v.SetConfigType("json")
		if err := v.ReadInConfig(); err != nil {
			log.Fatalf("failed to load config file %q: %v", configPath, err)
		}
	}

	// Флаги применяются поверх файла/default, но только если они были явно
	// переданы пользователем и соответствующая переменная окружения не задана —
	// иначе итоговое значение и так возьмётся viper из env-слоя (BindEnv выше),
	// что сохраняет приоритет "env > флаг".
	explicitFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { explicitFlags[f.Name] = true })

	setIfExplicit := func(flagName, key, envName string, val *string) {
		if explicitFlags[flagName] && os.Getenv(envName) == "" {
			v.Set(key, *val)
		}
	}
	setIfExplicit("a", "server_address", "SERVER_ADDRESS", flagServerURL)
	setIfExplicit("b", "base_url", "BASE_URL", flagResultURL)
	setIfExplicit("f", "file_storage_path", "FILE_STORAGE_PATH", flagFileStorage)
	setIfExplicit("d", "database_dsn", "DATABASE_DSN", flagDatabaseDSN)
	setIfExplicit("auth-secret", "auth_secret", "AUTH_SECRET", flagAuthSecret)
	setIfExplicit("audit-file", "audit_file", "AUDIT_FILE", flagAuditFile)
	setIfExplicit("audit-url", "audit_url", "AUDIT_URL", flagAuditURL)
	setIfExplicit("tls-cert", "tls_cert_file", "TLS_CERT_FILE", flagTLSCert)
	setIfExplicit("tls-key", "tls_key_file", "TLS_KEY_FILE", flagTLSKey)
	if explicitFlags["s"] && os.Getenv("ENABLE_HTTPS") == "" {
		v.Set("enable_https", *flagEnableHTTPS)
	}

	return &Config{
		BaseURL:         v.GetString("server_address"),
		ResultURL:       v.GetString("base_url"),
		FileStoragePath: v.GetString("file_storage_path"),
		DatabaseDSN:     v.GetString("database_dsn"),
		AuthSecret:      v.GetString("auth_secret"),
		AuditFile:       v.GetString("audit_file"),
		AuditURL:        v.GetString("audit_url"),
		EnableHTTPS:     v.GetBool("enable_https"),
		TLSCertFile:     v.GetString("tls_cert_file"),
		TLSKeyFile:      v.GetString("tls_key_file"),
	}
}
