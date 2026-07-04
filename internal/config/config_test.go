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

// --- Тесты LoadConfig с файлом конфигурации ---

func TestLoadConfig_FileConfig_AppliedWhenNoEnvOrFlag(t *testing.T) {
	// Значения из файла применяются, когда env и флаги не заданы
	path := writeConfigFile(t, `{
		"server_address": "localhost:9191",
		"base_url": "http://short.io",
		"enable_https": true
	}`)
	t.Setenv("CONFIG", path)

	resetFlags()
	cfg := LoadConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, "localhost:9191", cfg.BaseURL)
	assert.Equal(t, "http://short.io", cfg.ResultURL)
	assert.True(t, cfg.EnableHTTPS)
}

func TestLoadConfig_EnvOverridesFileConfig(t *testing.T) {
	// Переменная окружения перекрывает значение из файла конфигурации
	path := writeConfigFile(t, `{"server_address": "localhost:9191"}`)
	t.Setenv("CONFIG", path)
	t.Setenv("SERVER_ADDRESS", "localhost:4444")

	resetFlags()
	cfg := LoadConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, "localhost:4444", cfg.BaseURL)
}

func TestLoadConfig_FileConfigPath_ViaEnvCONFIG(t *testing.T) {
	// Путь к файлу конфигурации задается через переменную окружения CONFIG
	path := writeConfigFile(t, `{"auth_secret": "from-file-secret"}`)
	t.Setenv("CONFIG", path)

	resetFlags()
	cfg := LoadConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, "from-file-secret", cfg.AuthSecret)
}

func TestLoadConfig_EnableHTTPS_FileConfig(t *testing.T) {
	// enable_https: true из файла должно применяться
	path := writeConfigFile(t, `{"enable_https": true}`)
	t.Setenv("CONFIG", path)

	resetFlags()
	cfg := LoadConfig()
	require.NotNil(t, cfg)

	assert.True(t, cfg.EnableHTTPS)
}

func TestLoadConfig_EnableHTTPS_EnvOverridesFileConfig(t *testing.T) {
	// Переменная окружения ENABLE_HTTPS перекрывает enable_https из файла
	path := writeConfigFile(t, `{"enable_https": true}`)
	t.Setenv("CONFIG", path)
	t.Setenv("ENABLE_HTTPS", "false")

	resetFlags()
	cfg := LoadConfig()
	require.NotNil(t, cfg)

	assert.False(t, cfg.EnableHTTPS)
}

func TestLoadConfig_FileConfig_AllFields(t *testing.T) {
	// Все поля из файла конфигурации применяются корректно
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

	resetFlags()
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
		os.Setenv("CONFIG", "/nonexistent/config.json") //nolint:errcheck
		resetFlags()
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

func TestLoadConfig_InvalidJSONConfig_FatalExit(t *testing.T) {
	// Невалидный JSON в файле конфигурации должен завершить программу с ненулевым кодом
	if os.Getenv("TEST_FATAL_CONFIG") == "1" {
		dir, err := os.MkdirTemp("", "cfgtest")
		require.NoError(t, err)
		path := filepath.Join(dir, "config.json")
		require.NoError(t, os.WriteFile(path, []byte("{invalid json"), 0o600))
		os.Setenv("CONFIG", path) //nolint:errcheck
		resetFlags()
		LoadConfig()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadConfig_InvalidJSONConfig_FatalExit")
	cmd.Env = append(os.Environ(), "TEST_FATAL_CONFIG=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.NotEqual(t, 0, exitErr.ExitCode(), "expected non-zero exit code on invalid JSON config file")
}

func TestLoadConfig_FileConfigPath_ViaFlag(t *testing.T) {
	// Путь к файлу конфигурации может быть задан флагом -c (короткая форма)
	path := writeConfigFile(t, `{"server_address": "localhost:3131"}`)

	resetFlags()
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{origArgs[0], "-c", path}

	cfg := LoadConfig()

	assert.Equal(t, "localhost:3131", cfg.BaseURL)
}

func TestLoadConfig_FileConfigPath_ViaLongFlag(t *testing.T) {
	// Путь к файлу конфигурации может быть задан флагом -config (длинная форма)
	path := writeConfigFile(t, `{"server_address": "localhost:3232"}`)

	resetFlags()
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{origArgs[0], "-config", path}

	cfg := LoadConfig()

	assert.Equal(t, "localhost:3232", cfg.BaseURL)
}

func TestLoadConfig_FlagOverridesFileConfig(t *testing.T) {
	// Явно переданный флаг перекрывает значение из файла конфигурации (когда env не задана)
	path := writeConfigFile(t, `{"server_address": "localhost:9191", "enable_https": false}`)

	resetFlags()
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{origArgs[0], "-c", path, "-a", "localhost:2222", "-s"}

	cfg := LoadConfig()

	assert.Equal(t, "localhost:2222", cfg.BaseURL, "explicit flag must override file value")
	assert.True(t, cfg.EnableHTTPS, "explicit bool flag must override file value")
}

func TestLoadConfig_Defaults_NoSources(t *testing.T) {
	// Без файла, env и флагов должны применяться значения по умолчанию
	resetFlags()
	cfg := LoadConfig()

	assert.Equal(t, "http://localhost:8080", cfg.BaseURL)
	assert.Equal(t, "http://localhost:8080", cfg.ResultURL)
	assert.Equal(t, "shortener_data.json", cfg.FileStoragePath)
	assert.Equal(t, "dev_secret", cfg.AuthSecret)
	assert.Empty(t, cfg.DatabaseDSN)
	assert.Empty(t, cfg.AuditFile)
	assert.Empty(t, cfg.AuditURL)
	assert.False(t, cfg.EnableHTTPS)
}

// --- Существующие тесты HTTPS (без изменений) ---

func TestLoadConfig_EnableHTTPS_EnvTrue(t *testing.T) {
	t.Setenv("ENABLE_HTTPS", "true")

	resetFlags()
	cfg := LoadConfig()

	require.NotNil(t, cfg)
	assert.True(t, cfg.EnableHTTPS)
}

func TestLoadConfig_EnableHTTPS_Env1(t *testing.T) {
	t.Setenv("ENABLE_HTTPS", "1")

	resetFlags()
	cfg := LoadConfig()

	assert.True(t, cfg.EnableHTTPS)
}

func TestLoadConfig_EnableHTTPS_EnvFalse(t *testing.T) {
	t.Setenv("ENABLE_HTTPS", "false")

	resetFlags()
	cfg := LoadConfig()

	assert.False(t, cfg.EnableHTTPS)
}

func TestLoadConfig_EnableHTTPS_EnvEmpty(t *testing.T) {
	// ENABLE_HTTPS не задана — должен использоваться флаг (false по умолчанию)
	resetFlags()
	cfg := LoadConfig()

	assert.False(t, cfg.EnableHTTPS)
}

func TestLoadConfig_TLSCertKey_Env(t *testing.T) {
	t.Setenv("TLS_CERT_FILE", "/etc/ssl/cert.pem")
	t.Setenv("TLS_KEY_FILE", "/etc/ssl/key.pem")

	resetFlags()
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
	t.Setenv("ENABLE_HTTPS", "true")
	// Флаг -s по умолчанию false; env должна вернуть true
	resetFlags()
	cfg := LoadConfig()

	assert.True(t, cfg.EnableHTTPS, "env must override flag default")
}

func TestLoadConfig_EnvOverridesExplicitFlag(t *testing.T) {
	// env должна перекрывать даже явно переданный флаг (приоритет env > флаг > файл > default)
	t.Setenv("SERVER_ADDRESS", "localhost:5555")

	resetFlags()
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{origArgs[0], "-a", "localhost:1111"}

	cfg := LoadConfig()

	assert.Equal(t, "localhost:5555", cfg.BaseURL, "env must override an explicitly passed flag")
}
