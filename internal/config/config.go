// Package config загружает конфигурацию сервиса из переменных окружения,
// флагов командной строки и JSON-файла конфигурации с помощью github.com/spf13/viper.
// Приоритет: переменная окружения > флаг > файл конфигурации > значение по умолчанию.
//
// Флаги разбираются через github.com/spf13/pflag: однобуквенные мнемоники
// (-a, -b, -f, -d, -s, -c, -t) заданы как shorthand, путь к файлу конфигурации
// и доверенная подсеть дополнительно доступны в длинной форме (--config,
// --trusted-subnet), остальные флаги (--audit-file, --audit-url, --auth-secret,
// --tls-cert, --tls-key, --grpc-addr) существуют только в длинной форме через
// двойной дефис — это соответствует стандартной GNU/POSIX-конвенции
// pflag/Cobra (один дефис — короткие флаги, два — длинные).
package config

import (
	"log"
	"os"

	"github.com/spf13/pflag"
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
	// AuthSecret — секрет для подписи auth-куки HMAC-SHA256; флаг --auth-secret / env AUTH_SECRET.
	AuthSecret string
	// AuditFile — путь к файлу аудит-лога (JSON, один объект на строку); флаг --audit-file / env AUDIT_FILE.
	AuditFile string
	// AuditURL — URL удаленного приемника аудит-событий (HTTP POST); флаг --audit-url / env AUDIT_URL.
	AuditURL string
	// EnableHTTPS — включает режим HTTPS; флаг -s / env ENABLE_HTTPS.
	EnableHTTPS bool
	// TLSCertFile — путь к PEM-файлу сертификата TLS; флаг --tls-cert / env TLS_CERT_FILE.
	TLSCertFile string
	// TLSKeyFile — путь к PEM-файлу приватного ключа TLS; флаг --tls-key / env TLS_KEY_FILE.
	TLSKeyFile string
	// TrustedSubnet — строковое представление CIDR доверенной подсети для эндпоинта /api/internal/stats;
	// флаг -t / env TRUSTED_SUBNET. Пустое значение запрещает любой доступ к эндпоинту.
	TrustedSubnet string
	// GRPCAddress — адрес gRPC-сервера (например, ":3200"); флаг --grpc-addr / env GRPC_ADDRESS.
	// Пустое значение означает, что gRPC-сервер не запускается.
	GRPCAddress string
}

// setting описывает один параметр конфигурации: канонический ключ (совпадает с
// именем поля в JSON-файле конфигурации), длинное имя флага и имя переменной окружения.
type setting struct {
	key      string
	flagName string
	envName  string
}

var settings = []setting{
	{"server_address", "server_address", "SERVER_ADDRESS"},
	{"base_url", "base_url", "BASE_URL"},
	{"file_storage_path", "file_storage_path", "FILE_STORAGE_PATH"},
	{"database_dsn", "database_dsn", "DATABASE_DSN"},
	{"auth_secret", "auth-secret", "AUTH_SECRET"},
	{"audit_file", "audit-file", "AUDIT_FILE"},
	{"audit_url", "audit-url", "AUDIT_URL"},
	{"enable_https", "enable_https", "ENABLE_HTTPS"},
	{"tls_cert_file", "tls-cert", "TLS_CERT_FILE"},
	{"tls_key_file", "tls-key", "TLS_KEY_FILE"},
	{"trusted_subnet", "trusted-subnet", "TRUSTED_SUBNET"},
	{"grpc_address", "grpc-addr", "GRPC_ADDRESS"},
}

const envNameConfig = "CONFIG"

// LoadConfig читает конфигурацию из JSON-файла, переменных окружения и флагов командной строки.
// Путь к JSON-файлу задается флагом -c/--config или переменной окружения CONFIG.
// Приоритет источников: переменная окружения > флаг > файл конфигурации > значение по умолчанию.
// Должна вызываться один раз при старте приложения.
func LoadConfig() *Config {
	defaultURL := "http://localhost:8080"
	defaultFileStorage := "shortener_data.json"

	flagSet := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	flagConfigFile := flagSet.StringP("config", "c", "", "path to JSON config file")
	flagServerURL := flagSet.StringP("server_address", "a", "", "server base url")
	flagResultURL := flagSet.StringP("base_url", "b", "", "server result url")
	flagFileStorage := flagSet.StringP("file_storage_path", "f", "", "file storage path")
	flagDatabaseDSN := flagSet.StringP("database_dsn", "d", "", "database connection DSN")
	flagAuthSecret := flagSet.String("auth-secret", "", "auth secret for cookies")
	flagAuditFile := flagSet.String("audit-file", "", "path to audit log file")
	flagAuditURL := flagSet.String("audit-url", "", "URL of remote audit receiver")
	flagEnableHTTPS := flagSet.BoolP("enable_https", "s", false, "enable HTTPS mode")
	flagTLSCert := flagSet.String("tls-cert", "", "path to TLS certificate PEM file")
	flagTLSKey := flagSet.String("tls-key", "", "path to TLS private key PEM file")
	flagTrustedSubnet := flagSet.StringP("trusted-subnet", "t", "", "trusted subnet CIDR for /api/internal/stats")
	flagGRPCAddress := flagSet.String("grpc-addr", "", "gRPC server address (e.g. :3200); empty disables gRPC")

	// Игнорируем нераспознанные флаги (например, флаги тестового раннера go test),
	// вместо того чтобы завершать процесс с ошибкой.
	flagSet.ParseErrorsAllowlist = pflag.ParseErrorsAllowlist{UnknownFlags: true}
	if err := flagSet.Parse(os.Args[1:]); err != nil {
		log.Fatalf("failed to parse flags: %v", err)
	}

	v := viper.New()
	v.SetDefault("server_address", defaultURL)
	v.SetDefault("base_url", defaultURL)
	v.SetDefault("file_storage_path", defaultFileStorage)
	v.SetDefault("auth_secret", "dev_secret")

	for _, s := range settings {
		if err := v.BindEnv(s.key, s.envName); err != nil {
			log.Fatalf("failed to bind env %q: %v", s.envName, err)
		}
	}

	// Путь к файлу конфигурации: env CONFIG > флаг -c/--config
	configPath := os.Getenv(envNameConfig)
	if configPath == "" {
		configPath = *flagConfigFile
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
	setIfExplicit := func(s setting, val *string) {
		if flagSet.Changed(s.flagName) && os.Getenv(s.envName) == "" {
			v.Set(s.key, *val)
		}
	}
	setIfExplicit(settings[0], flagServerURL)
	setIfExplicit(settings[1], flagResultURL)
	setIfExplicit(settings[2], flagFileStorage)
	setIfExplicit(settings[3], flagDatabaseDSN)
	setIfExplicit(settings[4], flagAuthSecret)
	setIfExplicit(settings[5], flagAuditFile)
	setIfExplicit(settings[6], flagAuditURL)
	setIfExplicit(settings[8], flagTLSCert)
	setIfExplicit(settings[9], flagTLSKey)
	setIfExplicit(settings[10], flagTrustedSubnet)
	setIfExplicit(settings[11], flagGRPCAddress)
	if flagSet.Changed("enable_https") && os.Getenv("ENABLE_HTTPS") == "" {
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
		TrustedSubnet:   v.GetString("trusted_subnet"),
		GRPCAddress:     v.GetString("grpc_address"),
	}
}
