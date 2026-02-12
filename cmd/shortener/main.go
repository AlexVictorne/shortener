package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"

	"shortener/internal/config"
	"shortener/internal/handler"
	"shortener/internal/repository"
	"shortener/internal/service"
	"shortener/pkg/generator"
)

func main() {
	cfg := config.LoadConfig()

	store := repository.NewMemStorage()
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := store.Ping(ctx); err != nil {
		log.Fatalf("storage ping failed: %v", err)
	}

	idGenerator := generator.NewGenerator(8)

	service := service.NewTrimmerService(store, idGenerator, cfg.ResultURL)

	handler := handler.NewHandler(service)

	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	handler.SetupRoutes(r)

	serverAddr, err := url.Parse(cfg.BaseURL)
	if err != nil {
		log.Fatalf("Invalid server address: %v", err)
	}

	server := &http.Server{
		Addr:    serverAddr.Host,
		Handler: r,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on %s", serverAddr.String())
		log.Printf("Result link direct to: %s", cfg.ResultURL)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop

	log.Println("Shutdown server...")

	// Graceful shutdown
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
