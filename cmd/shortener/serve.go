package main

import (
	"log"
	"net/http"

	"shortener/internal/config"
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
