package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"shortener/internal/config"
	"shortener/internal/handler"
	"shortener/internal/handler/options"
	"shortener/internal/repository"
	"shortener/internal/service"
	"shortener/pkg/audit"
	"shortener/pkg/buildinfo"
	"shortener/pkg/generator"
	"shortener/pkg/validator"
)

var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

func main() {
	fmt.Println(buildinfo.Format(buildVersion, buildDate, buildCommit))

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	zerolog.TimeFieldFormat = time.RFC3339Nano

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

	var store repository.Storage
	var pgStore *repository.PgStorage

	if cfg.DatabaseDSN != "" {
		pgStore, err = repository.NewPgStorage(context.Background(), cfg.DatabaseDSN)
		if err != nil {
			log.Fatalf("PgStorage initialization error: %v", err)
		}
		log.Println("Using PostgreSQL storage (PgStorage)")
		store = pgStore
	} else {
		memStore, err := repository.NewMemStorageWithFile(cfg.FileStoragePath)
		if err != nil {
			log.Fatalf("Storage initialization error: %v", err)
		}
		log.Println("Using in-memory storage (MemStorage)")
		store = memStore
	}
	defer store.Close()

	idGenerator := generator.NewGenerator(8)

	service := service.NewTrimmerService(store, idGenerator, validatedResultURL)

	auditLogger := zerolog.New(os.Stderr).With().Str("component", "audit").Timestamp().Logger()
	auditor, err := audit.Build(cfg.AuditFile, cfg.AuditURL, auditLogger)
	if err != nil {
		log.Fatalf("audit initialization error: %v", err)
	}
	defer auditor.Close()

	handlerOpts := []options.OptHandlerOptionsSetter{
		options.WithAuthSecret(cfg.AuthSecret),
		options.WithAuditor(auditor),
	}
	if pgStore != nil {
		handlerOpts = append(handlerOpts, options.WithPinger(pgStore))
	}

	handlerInstance := handler.NewHandler(service, handlerOpts...)
	r := chi.NewRouter()
	handlerInstance.SetupRoutes(r)

	serverAddr, err := url.Parse(validatedBaseURL)
	if err != nil {
		log.Fatalf("Invalid server address: %v", err)
	}

	server := &http.Server{
		Addr:    serverAddr.Host,
		Handler: r,
	}

	go func() {
		log.Println("pprof server on 127.0.0.1:6060")
		if err := http.ListenAndServe("127.0.0.1:6060", nil); err != nil {
			log.Printf("pprof server error: %v", err)
		}
	}()

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
