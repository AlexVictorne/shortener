package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shortener/internal/config"
)

// generateSelfSignedCert создает временные PEM-файлы самоподписанного сертификата
// и возвращает пути к ним. Файлы удаляются автоматически по завершении теста.
func generateSelfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	dir := t.TempDir()

	// Генерируем ключевую пару ECDSA P-256
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// Создаем шаблон самоподписанного сертификата
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)

	// Записываем сертификат в PEM-файл
	certFile = filepath.Join(dir, "cert.pem")
	cf, err := os.Create(certFile)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	cf.Close()

	// Записываем приватный ключ в PEM-файл
	keyFile = filepath.Join(dir, "key.pem")
	kf, err := os.Create(keyFile)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(kf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	kf.Close()

	return certFile, keyFile
}

// freeAddr выделяет свободный порт на loopback-адресе и возвращает его адрес.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	l.Close()
	return addr
}

func TestListenAndServe_HTTP(t *testing.T) {
	addr := freeAddr(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	cfg := &config.Config{EnableHTTPS: false}

	// Запускаем сервер в фоновой горутине
	errCh := make(chan error, 1)
	go func() {
		errCh <- listenAndServe(srv, cfg)
	}()

	// Ждем, пока сервер поднимется
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + addr + "/")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond)

	// Останавливаем сервер
	srv.Close()
	err := <-errCh
	assert.ErrorIs(t, err, http.ErrServerClosed)
}

func TestListenAndServe_HTTPS(t *testing.T) {
	addr := freeAddr(t)
	certFile, keyFile := generateSelfSignedCert(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("tls-ok")) //nolint:errcheck
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	cfg := &config.Config{
		EnableHTTPS: true,
		TLSCertFile: certFile,
		TLSKeyFile:  keyFile,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- listenAndServe(srv, cfg)
	}()

	// Клиент, доверяющий самоподписанному сертификату
	tlsCfg := &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}}

	require.Eventually(t, func() bool {
		resp, err := client.Get("https://" + addr + "/")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode == http.StatusOK && string(body) == "tls-ok"
	}, 3*time.Second, 50*time.Millisecond)

	srv.Close()
	err := <-errCh
	assert.ErrorIs(t, err, http.ErrServerClosed)
}

func TestListenAndServe_HTTPS_BadCert(t *testing.T) {
	addr := freeAddr(t)

	srv := &http.Server{Addr: addr, Handler: http.NewServeMux()}
	cfg := &config.Config{
		EnableHTTPS: true,
		TLSCertFile: "/nonexistent/cert.pem",
		TLSKeyFile:  "/nonexistent/key.pem",
	}

	// Должна вернуть ошибку немедленно (файлы не существуют)
	err := listenAndServe(srv, cfg)
	assert.Error(t, err)
}
