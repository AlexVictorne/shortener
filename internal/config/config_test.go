package config

import (
	"flag"
	"io"
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
