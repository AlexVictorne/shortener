package main

import (
	"context"
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

// main инициализирует конфигурацию, хранилище, сервисы и запускает HTTP-сервер.
// Завершение выполняется корректно при получении SIGINT, SIGTERM или SIGQUIT:
// все активные запросы дообрабатываются, несохраненные данные сбрасываются в хранилище.
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

	idGenerator := generator.NewGenerator(8)

	service := service.NewTrimmerService(store, idGenerator, validatedResultURL)

	auditLogger := zerolog.New(os.Stderr).With().Str("component", "audit").Timestamp().Logger()
	auditor, err := audit.Build(cfg.AuditFile, cfg.AuditURL, auditLogger)
	if err != nil {
		log.Fatalf("audit initialization error: %v", err)
	}

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

	// Перехватываем SIGINT, SIGTERM и SIGQUIT для корректного завершения
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	// onShutdown вызывается гарантированно при любом завершении сервера:
	// сохраняет данные и освобождает ресурсы
	onShutdown := func() {
		store.Close()
		auditor.Close()
	}

	if err := run(ctx, server, cfg, onShutdown); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
