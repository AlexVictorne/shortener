package config

import (
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetFlags сбрасывает глобальный flag.CommandLine перед каждым тестом,
// чтобы повторный вызов flag.Parse() не вызывал ошибку «flag redefined».
// Вывод направляется в io.Discard, чтобы скрыть «flag provided but not defined»
// для внутренних флагов тестового runner.
func resetFlags() {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	flag.CommandLine = fs
}

// writeConfigFile создает временный JSON-файл конфигурации в директории t.TempDir()
// и возвращает путь к нему.
func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// --- Тесты loadFileConfig ---

func TestLoadFileConfig_EmptyPath(t *testing.T) {
	// Пустой путь должен вернуть nil без ошибки
	fc, err := loadFileConfig("")
	require.NoError(t, err)
	assert.Nil(t, fc)
}

func TestLoadFileConfig_ValidFile(t *testing.T) {
	// Корректный JSON-файл читается без ошибок, все поля заполнены
	path := writeConfigFile(t, `{
		"server_address": "localhost:9090",
		"base_url": "http://short.example.com",
		"file_storage_path": "/tmp/data.json",
		"database_dsn": "postgres://user:pass@localhost/db",
		"auth_secret": "supersecret",
		"audit_file": "/var/log/audit.json",
		"audit_url": "http://audit.example.com",
		"enable_https": true,
		"tls_cert_file": "/etc/ssl/cert.pem",
		"tls_key_file": "/etc/ssl/key.pem"
	}`)

	fc, err := loadFileConfig(path)
	require.NoError(t, err)
	require.NotNil(t, fc)

	assert.Equal(t, "localhost:9090", *fc.ServerAddress)
	assert.Equal(t, "http://short.example.com", *fc.BaseURL)
	assert.Equal(t, "/tmp/data.json", *fc.FileStoragePath)
	assert.Equal(t, "postgres://user:pass@localhost/db", *fc.DatabaseDSN)
	assert.Equal(t, "supersecret", *fc.AuthSecret)
	assert.Equal(t, "/var/log/audit.json", *fc.AuditFile)
	assert.Equal(t, "http://audit.example.com", *fc.AuditURL)
	assert.True(t, *fc.EnableHTTPS)
	assert.Equal(t, "/etc/ssl/cert.pem", *fc.TLSCertFile)
	assert.Equal(t, "/etc/ssl/key.pem", *fc.TLSKeyFile)
}

func TestLoadFileConfig_PartialFile(t *testing.T) {
	// JSON-файл с частичными полями: незаданные поля должны быть nil
	path := writeConfigFile(t, `{"server_address": "localhost:7777"}`)

	fc, err := loadFileConfig(path)
	require.NoError(t, err)
	require.NotNil(t, fc)

	assert.Equal(t, "localhost:7777", *fc.ServerAddress)
	assert.Nil(t, fc.BaseURL)
	assert.Nil(t, fc.EnableHTTPS)
}

func TestLoadFileConfig_InvalidJSON(t *testing.T) {
	// Невалидный JSON должен вернуть ошибку
	path := writeConfigFile(t, `{invalid json}`)

	fc, err := loadFileConfig(path)
	assert.Error(t, err)
	assert.Nil(t, fc)
}

func TestLoadFileConfig_MissingFile(t *testing.T) {
	// Несуществующий файл должен вернуть ошибку
	fc, err := loadFileConfig("/nonexistent/path/config.json")
	assert.Error(t, err)
	assert.Nil(t, fc)
}

// --- Тесты resolveString ---

func TestResolveString_EnvHasPriority(t *testing.T) {
	fileVal := "from-file"
	result := resolveString("from-env", "from-flag", &fileVal, "default")
	assert.Equal(t, "from-env", result)
}

func TestResolveString_FlagOverridesFile(t *testing.T) {
	fileVal := "from-file"
	result := resolveString("", "from-flag", &fileVal, "default")
	assert.Equal(t, "from-flag", result)
}

func TestResolveString_FileOverridesDefault(t *testing.T) {
	fileVal := "from-file"
	result := resolveString("", "", &fileVal, "default")
	assert.Equal(t, "from-file", result)
}

func TestResolveString_DefaultWhenAllEmpty(t *testing.T) {
	result := resolveString("", "", nil, "default")
	assert.Equal(t, "default", result)
}

// --- Тесты resolveBool ---

func TestResolveBool_EnvHasPriority(t *testing.T) {
	fileVal := false
	result := resolveBool("true", false, false, &fileVal, false)
	assert.True(t, result)
}

func TestResolveBool_ExplicitFlagOverridesFile(t *testing.T) {
	fileVal := false
	result := resolveBool("", true, true, &fileVal, false)
	assert.True(t, result)
}

func TestResolveBool_FileOverridesDefault(t *testing.T) {
	fileVal := true
	result := resolveBool("", false, false, &fileVal, false)
	assert.True(t, result)
}

func TestResolveBool_DefaultWhenAllAbsent(t *testing.T) {
	result := resolveBool("", false, false, nil, true)
	assert.True(t, result)
}

// --- Тесты LoadConfig с файлом конфигурации ---

func TestLoadConfig_FileConfig_AppliedWhenNoEnvOrFlag(t *testing.T) {
	// Значения из файла применяются, когда env и флаги не заданы
	resetFlags()
	path := writeConfigFile(t, `{
		"server_address": "localhost:9191",
		"base_url": "http://short.io",
		"enable_https": true
	}`)
	t.Setenv("CONFIG", path)

	cfg := LoadConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, "localhost:9191", cfg.BaseURL)
	assert.Equal(t, "http://short.io", cfg.ResultURL)
	assert.True(t, cfg.EnableHTTPS)
}

func TestLoadConfig_EnvOverridesFileConfig(t *testing.T) {
	// Переменная окружения перекрывает значение из файла конфигурации
	resetFlags()
	path := writeConfigFile(t, `{"server_address": "localhost:9191"}`)
	t.Setenv("CONFIG", path)
	t.Setenv("SERVER_ADDRESS", "localhost:4444")

	cfg := LoadConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, "localhost:4444", cfg.BaseURL)
}

func TestLoadConfig_FileConfigPath_ViaEnvCONFIG(t *testing.T) {
	// Путь к файлу конфигурации задается через переменную окружения CONFIG
	resetFlags()
	path := writeConfigFile(t, `{"auth_secret": "from-file-secret"}`)
	t.Setenv("CONFIG", path)

	cfg := LoadConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, "from-file-secret", cfg.AuthSecret)
}

func TestLoadConfig_EnableHTTPS_FileConfig(t *testing.T) {
	// enable_https: true из файла должно применяться
	resetFlags()
	path := writeConfigFile(t, `{"enable_https": true}`)
	t.Setenv("CONFIG", path)

	cfg := LoadConfig()
	require.NotNil(t, cfg)

	assert.True(t, cfg.EnableHTTPS)
}

func TestLoadConfig_EnableHTTPS_EnvOverridesFileConfig(t *testing.T) {
	// Переменная окружения ENABLE_HTTPS перекрывает enable_https из файла
	resetFlags()
	path := writeConfigFile(t, `{"enable_https": true}`)
	t.Setenv("CONFIG", path)
	t.Setenv("ENABLE_HTTPS", "false")

	cfg := LoadConfig()
	require.NotNil(t, cfg)

	assert.False(t, cfg.EnableHTTPS)
}

func TestLoadConfig_FileConfig_AllFields(t *testing.T) {
	// Все поля из файла конфигурации применяются корректно
	resetFlags()
	path := writeConfigFile(t, `{
		"server_address": "0.0.0.0:8888",
		"base_url": "https://s.io",
		"file_storage_path": "/tmp/store.json",
		"database_dsn": "postgres://localhost/shortener",
		"auth_secret": "my-secret",
		"audit_file": "/tmp/audit.log",
		"audit_url": "http://audit.io/events",
		"enable_https": false,
		"tls_cert_file": "/tmp/cert.pem",
		"tls_key_file": "/tmp/key.pem"
	}`)
	t.Setenv("CONFIG", path)

	cfg := LoadConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, "0.0.0.0:8888", cfg.BaseURL)
	assert.Equal(t, "https://s.io", cfg.ResultURL)
	assert.Equal(t, "/tmp/store.json", cfg.FileStoragePath)
	assert.Equal(t, "postgres://localhost/shortener", cfg.DatabaseDSN)
	assert.Equal(t, "my-secret", cfg.AuthSecret)
	assert.Equal(t, "/tmp/audit.log", cfg.AuditFile)
	assert.Equal(t, "http://audit.io/events", cfg.AuditURL)
	assert.False(t, cfg.EnableHTTPS)
	assert.Equal(t, "/tmp/cert.pem", cfg.TLSCertFile)
	assert.Equal(t, "/tmp/key.pem", cfg.TLSKeyFile)
}

func TestLoadConfig_FileConfig_MissingFile_FatalExit(t *testing.T) {
	// Несуществующий файл конфигурации должен завершить программу с ненулевым кодом
	if os.Getenv("TEST_FATAL_CONFIG") == "1" {
		resetFlags()
		// Устанавливаем несуществующий путь напрямую
		os.Setenv("CONFIG", "/nonexistent/config.json") //nolint:errcheck
		LoadConfig()
		return
	}
	// Запускаем подпроцесс, чтобы поймать log.Fatal
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadConfig_FileConfig_MissingFile_FatalExit")
	cmd.Env = append(os.Environ(), "TEST_FATAL_CONFIG=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.NotEqual(t, 0, exitErr.ExitCode(), "expected non-zero exit code on missing config file")
}

// --- Существующие тесты HTTPS (без изменений) ---

func TestLoadConfig_EnableHTTPS_EnvTrue(t *testing.T) {
	resetFlags()
	t.Setenv("ENABLE_HTTPS", "true")

	cfg := LoadConfig()

	require.NotNil(t, cfg)
	assert.True(t, cfg.EnableHTTPS)
}

func TestLoadConfig_EnableHTTPS_Env1(t *testing.T) {
	resetFlags()
	t.Setenv("ENABLE_HTTPS", "1")

	cfg := LoadConfig()

	assert.True(t, cfg.EnableHTTPS)
}

func TestLoadConfig_EnableHTTPS_EnvFalse(t *testing.T) {
	resetFlags()
	t.Setenv("ENABLE_HTTPS", "false")

	cfg := LoadConfig()

	assert.False(t, cfg.EnableHTTPS)
}

func TestLoadConfig_EnableHTTPS_EnvEmpty(t *testing.T) {
	resetFlags()
	// ENABLE_HTTPS не задана — должен использоваться флаг (false по умолчанию)
	cfg := LoadConfig()

	assert.False(t, cfg.EnableHTTPS)
}

func TestLoadConfig_TLSCertKey_Env(t *testing.T) {
	resetFlags()
	t.Setenv("TLS_CERT_FILE", "/etc/ssl/cert.pem")
	t.Setenv("TLS_KEY_FILE", "/etc/ssl/key.pem")

	cfg := LoadConfig()

	assert.Equal(t, "/etc/ssl/cert.pem", cfg.TLSCertFile)
	assert.Equal(t, "/etc/ssl/key.pem", cfg.TLSKeyFile)
}

func TestLoadConfig_TLSCertKey_Defaults(t *testing.T) {
	resetFlags()

	cfg := LoadConfig()

	assert.Empty(t, cfg.TLSCertFile)
	assert.Empty(t, cfg.TLSKeyFile)
}

func TestLoadConfig_EnableHTTPS_EnvOverridesFlag(t *testing.T) {
	// env ENABLE_HTTPS=true должна перекрывать значение флага по умолчанию
	resetFlags()
	t.Setenv("ENABLE_HTTPS", "true")
	// Флаг -tls по умолчанию false; env должна вернуть true
	cfg := LoadConfig()

	assert.True(t, cfg.EnableHTTPS, "env must override flag default")
}
