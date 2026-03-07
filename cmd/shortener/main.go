package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"shortener/internal/config"
	"shortener/internal/handler"
	"shortener/internal/repository"
	"shortener/internal/service"
	"shortener/pkg/generator"
	"shortener/pkg/validator"
)

func main() {
	cfg := config.LoadConfig()

	// Валидация параметров конфигурации рядом с использованием
	validatedBaseURL, err := validator.ValidateURL(cfg.BaseURL)
	if err != nil {
		log.Fatalf("BaseURL validation error: %v", err)
	}
	validatedResultURL, err := validator.ValidateURL(cfg.ResultURL)
	if err != nil {
		log.Fatalf("ResultURL validation error: %v", err)
	}

	store := repository.NewMemStorage()
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	idGenerator := generator.NewGenerator(8)

	service := service.NewTrimmerService(store, idGenerator, validatedResultURL)

	handler := handler.NewHandler(service)

	r := chi.NewRouter()

	handler.SetupRoutes(r)

	serverAddr, err := url.Parse(validatedBaseURL)
	if err != nil {
		log.Fatalf("Invalid server address: %v", err)
	}

	server := &http.Server{
		Addr:    serverAddr.Host,
		Handler: r,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errChan := make(chan error, 1)

	go func() {
		log.Printf("Server starting on %s", serverAddr.String())
		log.Printf("Result link direct to: %s", cfg.ResultURL)
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		} else {
			errChan <- nil
		}
	}()

	select {
	case <-ctx.Done():
		log.Println("Shutdown server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server shutdown error: %v", err)
		}
		log.Println("Server stopped")
	case err := <-errChan:
		if err != nil {
			stop()
			log.Fatalf("Server error: %v", err)
		}
	}
}
