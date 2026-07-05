package main

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"shortener/internal/config"
)

// freePort возвращает свободный TCP-порт на localhost.
func freePort(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// testConfig возвращает минимальную конфигурацию для тестов.
func testConfig(addr string) *config.Config {
	return &config.Config{
		BaseURL:     "http://" + addr,
		ResultURL:   "http://" + addr,
		EnableHTTPS: false,
	}
}

// TestRun_GracefulShutdownOnContextCancel проверяет, что сервер корректно останавливается
// после отмены контекста (эмуляция сигнала завершения).
func TestRun_GracefulShutdownOnContextCancel(t *testing.T) {
	addr := freePort(t)
	server := &http.Server{
		Addr:    addr,
		Handler: http.NewServeMux(),
	}
	cfg := testConfig(addr)

	ctx, cancel := context.WithCancel(context.Background())

	// Даем серверу время подняться, затем отменяем контекст
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := run(ctx, servers{http: server}, cfg, func() {})
	if err != nil {
		t.Errorf("expected nil error on clean shutdown, got: %v", err)
	}
}

// TestRun_OnShutdownCalledOnSignal проверяет, что колбэк onShutdown вызывается
// при получении сигнала завершения (отмена контекста).
func TestRun_OnShutdownCalledOnSignal(t *testing.T) {
	addr := freePort(t)
	server := &http.Server{
		Addr:    addr,
		Handler: http.NewServeMux(),
	}
	cfg := testConfig(addr)

	ctx, cancel := context.WithCancel(context.Background())

	var called atomic.Bool

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_ = run(ctx, servers{http: server}, cfg, func() {
		called.Store(true)
	})

	if !called.Load() {
		t.Error("onShutdown was not called after shutdown signal")
	}
}

// TestRun_OnShutdownCalledOnStartupError проверяет, что колбэк onShutdown вызывается
// при ошибке запуска сервера (занятый порт).
func TestRun_OnShutdownCalledOnStartupError(t *testing.T) {
	addr := freePort(t)

	// Занимаем порт, чтобы сервер не смог запуститься
	blocker, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("failed to bind blocker: %v", err)
	}
	defer blocker.Close()

	server := &http.Server{
		Addr:    addr,
		Handler: http.NewServeMux(),
	}
	cfg := testConfig(addr)

	ctx := context.Background()

	var called atomic.Bool

	err = run(ctx, servers{http: server}, cfg, func() {
		called.Store(true)
	})

	if err == nil {
		t.Error("expected error when port is already in use")
	}
	if !called.Load() {
		t.Error("onShutdown was not called after startup error")
	}
}

// TestRun_InFlightRequestCompletes проверяет, что запрос, начатый до получения сигнала,
// успевает завершиться до остановки сервера.
func TestRun_InFlightRequestCompletes(t *testing.T) {
	addr := freePort(t)

	// Канал для синхронизации: хэндлер сигнализирует о начале обработки запроса
	handlerStarted := make(chan struct{})
	handlerDone := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		close(handlerStarted)
		// Имитируем долгий запрос
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		close(handlerDone)
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	cfg := testConfig(addr)

	ctx, cancel := context.WithCancel(context.Background())

	runDone := make(chan error, 1)
	go func() {
		runDone <- run(ctx, servers{http: server}, cfg, func() {})
	}()

	// Ждем, пока сервер поднимется
	time.Sleep(50 * time.Millisecond)

	// Отправляем медленный запрос
	go func() {
		resp, err := http.Get("http://" + addr + "/slow") //nolint:noctx
		if err == nil {
			resp.Body.Close()
		}
	}()

	// Ждем начала обработки запроса, затем отправляем сигнал завершения
	<-handlerStarted
	cancel()

	// Запрос должен завершиться до остановки сервера
	select {
	case <-handlerDone:
		// Ожидаемый путь
	case <-time.After(5 * time.Second):
		t.Error("in-flight request did not complete before server stopped")
	}

	// run должен завершиться без ошибки
	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("expected nil error on graceful shutdown, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("run did not return after shutdown")
	}
}

// TestRun_GRPCGracefulShutdownOnContextCancel проверяет, что при наличии gRPC-сервера
// он поднимается вместе с HTTP-сервером и корректно останавливается по сигналу завершения,
// симметрично поведению HTTP-сервера.
func TestRun_GRPCGracefulShutdownOnContextCancel(t *testing.T) {
	httpAddr := freePort(t)
	grpcAddr := freePort(t)

	server := &http.Server{
		Addr:    httpAddr,
		Handler: http.NewServeMux(),
	}
	cfg := testConfig(httpAddr)
	grpcSrv := grpc.NewServer()

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := run(ctx, servers{http: server, grpc: grpcSrv, grpcAddr: grpcAddr}, cfg, func() {})
	if err != nil {
		t.Errorf("expected nil error on clean shutdown, got: %v", err)
	}
}

// TestRun_GRPCStartupErrorStopsHTTPServer проверяет, что ошибка запуска gRPC-сервера
// (например, занятый порт) приводит к остановке уже запущенного HTTP-сервера
// и вызову onShutdown — симметрично тому, как ошибка запуска HTTP-сервера
// останавливает весь run().
func TestRun_GRPCStartupErrorStopsHTTPServer(t *testing.T) {
	httpAddr := freePort(t)
	grpcAddr := freePort(t)

	// Занимаем порт gRPC-сервера, чтобы он не смог запуститься
	blocker, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		t.Fatalf("failed to bind blocker: %v", err)
	}
	defer blocker.Close()

	server := &http.Server{
		Addr:    httpAddr,
		Handler: http.NewServeMux(),
	}
	cfg := testConfig(httpAddr)
	grpcSrv := grpc.NewServer()

	ctx := context.Background()

	var called atomic.Bool

	err = run(ctx, servers{http: server, grpc: grpcSrv, grpcAddr: grpcAddr}, cfg, func() {
		called.Store(true)
	})

	if err == nil {
		t.Error("expected error when gRPC port is already in use")
	}
	if !called.Load() {
		t.Error("onShutdown was not called after gRPC startup error")
	}
}

// TestRun_PprofServerStoppedOnShutdown проверяет, что pprof-сервер поднимается вместе
// с основным HTTP-сервером и перестает отвечать после сигнала завершения — то есть
// участвует в общем graceful shutdown lifecycle, симметрично основному серверу.
func TestRun_PprofServerStoppedOnShutdown(t *testing.T) {
	httpAddr := freePort(t)
	pprofAddr := freePort(t)

	server := &http.Server{
		Addr:    httpAddr,
		Handler: http.NewServeMux(),
	}
	pprofServer := &http.Server{
		Addr:    pprofAddr,
		Handler: http.NewServeMux(),
	}
	cfg := testConfig(httpAddr)

	ctx, cancel := context.WithCancel(context.Background())

	runDone := make(chan error, 1)
	go func() {
		runDone <- run(ctx, servers{http: server, pprof: pprofServer}, cfg, func() {})
	}()

	// Ждем, пока pprof-сервер поднимется, и убеждаемся, что порт слушается.
	// Соединение сразу закрываем, иначе Shutdown будет ждать его завершения.
	time.Sleep(50 * time.Millisecond)
	conn, err := net.Dial("tcp", pprofAddr)
	if err != nil {
		t.Fatalf("expected pprof server to be listening, got: %v", err)
	}
	conn.Close()

	cancel()

	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("expected nil error on graceful shutdown, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after shutdown")
	}

	if _, err := net.Dial("tcp", pprofAddr); err == nil {
		t.Error("expected pprof server to stop listening after shutdown")
	}
}
