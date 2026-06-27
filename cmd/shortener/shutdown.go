package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"shortener/internal/config"
)

// shutdownTimeout — максимальное время ожидания завершения активных запросов.
const shutdownTimeout = 10 * time.Second

// run запускает HTTP-сервер и блокируется до получения сигнала завершения
// или ошибки запуска. После сигнала выполняет graceful shutdown:
// ждет завершения активных запросов, затем вызывает onShutdown для сохранения
// несохраненных данных. onShutdown вызывается гарантированно — даже при ошибке shutdown.
func run(ctx context.Context, server *http.Server, cfg *config.Config, onShutdown func()) error {
	// Канал для получения ошибки запуска сервера
	errChan := make(chan error, 1)

	go func() {
		log.Printf("Server starting on %s (HTTPS: %v)", server.Addr, cfg.EnableHTTPS)
		log.Printf("Result link direct to: %s", cfg.ResultURL)

		err := listenAndServe(server, cfg)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		} else {
			errChan <- nil
		}
	}()

	var runErr error

	select {
	case <-ctx.Done():
		// Получен сигнал завершения — запускаем graceful shutdown
		log.Println("Shutdown signal received, stopping server...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// Не используем log.Fatalf — он вызывает os.Exit и пропускает onShutdown
			log.Printf("Server shutdown error: %v", err)
			runErr = err
		}

		log.Println("Server stopped")

	case err := <-errChan:
		// Сервер завершился до получения сигнала (ошибка запуска)
		if err != nil {
			runErr = err
		}
	}

	// Сохраняем данные и закрываем ресурсы гарантированно,
	// независимо от того, была ли ошибка shutdown
	onShutdown()

	return runErr
}
