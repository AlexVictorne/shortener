package main

import (
	"fmt"
	"log"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"shortener/internal/config"
	"shortener/internal/grpchandler"
	"shortener/internal/pb"
	"shortener/internal/service"
	"shortener/pkg/audit"
)

// listenAndServe запускает HTTP или HTTPS-сервер в зависимости от конфигурации.
// При cfg.EnableHTTPS == true вызывает server.ListenAndServeTLS с сертификатом
// cfg.TLSCertFile и ключом cfg.TLSKeyFile.
// При cfg.EnableHTTPS == false вызывает server.ListenAndServe (обычный HTTP).
func listenAndServe(server *http.Server, cfg *config.Config) error {
	if cfg.EnableHTTPS {
		log.Printf("Starting HTTPS server on %s", server.Addr)
		return server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
	}
	log.Printf("Starting HTTP server on %s", server.Addr)
	return server.ListenAndServe()
}

// newGRPCServer конструирует gRPC-сервер с зарегистрированным ShortenerServiceServer.
// При cfg.EnableHTTPS == true сервер использует те же сертификат и ключ, что и основной
// HTTP-сервер (cfg.TLSCertFile/cfg.TLSKeyFile) — симметрично тому, как listenAndServe
// выбирает между ListenAndServe и ListenAndServeTLS для основного сервера.
// При cfg.EnableHTTPS == false gRPC-сервер обслуживает соединения в открытом виде.
func newGRPCServer(cfg *config.Config, svc *service.TrimmerService, auditor audit.Auditor) (*grpc.Server, error) {
	var opts []grpc.ServerOption
	if cfg.EnableHTTPS {
		creds, err := credentials.NewServerTLSFromFile(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("gRPC TLS credentials error: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	srv := grpc.NewServer(opts...)
	pb.RegisterShortenerServiceServer(srv, grpchandler.NewServer(svc, cfg.AuthSecret, auditor))
	return srv, nil
}
