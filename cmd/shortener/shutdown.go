package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"

	"shortener/internal/config"
)

// shutdownTimeout — максимальное время ожидания завершения активных запросов
// как для HTTP, так и для gRPC сервера.
const shutdownTimeout = 10 * time.Second

// run запускает основной HTTP-сервер, вспомогательный pprof-сервер (если pprofServer
// не nil) и gRPC-сервер (если grpcSrv не nil), затем блокируется до получения сигнала
// завершения или ошибки запуска основного HTTP- или gRPC-сервера. После сигнала
// выполняет graceful shutdown всех серверов: ждет завершения активных запросов, затем
// вызывает onShutdown для сохранения несохраненных данных. onShutdown вызывается
// гарантированно — даже при ошибке shutdown.
//
// Ошибка запуска pprof-сервера не останавливает остальные серверы и не возвращается
// из run — это вспомогательный отладочный сервер, его недоступность не критична
// для работы сервиса; но при остановке он гасится вместе со всеми остальными.
func run(ctx context.Context, server *http.Server, pprofServer *http.Server, grpcSrv *grpc.Server, grpcAddr string, cfg *config.Config, onShutdown func()) error {
	// Каналы для получения ошибок запуска серверов
	httpErrChan := make(chan error, 1)

	go func() {
		log.Printf("Server starting on %s (HTTPS: %v)", server.Addr, cfg.EnableHTTPS)
		log.Printf("Result link direct to: %s", cfg.ResultURL)

		err := listenAndServe(server, cfg)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			httpErrChan <- err
		} else {
			httpErrChan <- nil
		}
	}()

	if pprofServer != nil {
		go func() {
			log.Printf("pprof server starting on %s", pprofServer.Addr)

			if err := pprofServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("pprof server error: %v", err)
			}
		}()
	}

	// grpcErrChan остается nil, если gRPC-сервер не сконфигурирован — получение
	// из nil-канала блокируется навсегда, поэтому select ниже просто игнорирует эту ветку.
	var grpcErrChan chan error
	if grpcSrv != nil {
		grpcErrChan = make(chan error, 1)

		go func() {
			lis, err := net.Listen("tcp", grpcAddr)
			if err != nil {
				grpcErrChan <- err
				return
			}

			log.Printf("gRPC server starting on %s (TLS: %v)", grpcAddr, cfg.EnableHTTPS)

			if err := grpcSrv.Serve(lis); err != nil {
				grpcErrChan <- err
			} else {
				grpcErrChan <- nil
			}
		}()
	}

	var runErr error

	select {
	case <-ctx.Done():
		// Получен сигнал завершения — запускаем graceful shutdown
		log.Println("Shutdown signal received, stopping servers...")

		if err := stopHTTPServer(server, "HTTP server"); err != nil {
			runErr = err
		}

		stopHTTPServer(pprofServer, "pprof server") //nolint:errcheck // недоступность отладочного сервера не критична
		stopGRPCServer(grpcSrv)

	case err := <-httpErrChan:
		// HTTP-сервер завершился до получения сигнала (ошибка запуска)
		if err != nil {
			runErr = err
		}
		stopHTTPServer(pprofServer, "pprof server") //nolint:errcheck // недоступность отладочного сервера не критична
		stopGRPCServer(grpcSrv)

	case err := <-grpcErrChan:
		// gRPC-сервер завершился до получения сигнала (ошибка запуска)
		if err != nil {
			runErr = err
		}

		stopHTTPServer(server, "HTTP server")       //nolint:errcheck // ошибку остановки основного сервера уже некуда возвращать содержательно
		stopHTTPServer(pprofServer, "pprof server") //nolint:errcheck // недоступность отладочного сервера не критична
	}

	// Сохраняем данные и закрываем ресурсы гарантированно,
	// независимо от того, была ли ошибка shutdown
	onShutdown()

	return runErr
}

// stopHTTPServer останавливает переданный HTTP-сервер с graceful shutdown не дольше
// shutdownTimeout. name используется только для логирования. Если server равен nil,
// не делает ничего — это позволяет безусловно вызывать функцию для опциональных
// серверов вроде pprof.
func stopHTTPServer(server *http.Server, name string) error {
	if server == nil {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		// Не используем log.Fatalf — он вызывает os.Exit и пропускает onShutdown
		log.Printf("%s shutdown error: %v", name, err)
		return err
	}

	log.Printf("%s stopped", name)
	return nil
}

// stopGRPCServer останавливает gRPC-сервер, ожидая завершения активных RPC не дольше
// shutdownTimeout — симметрично таймауту graceful shutdown HTTP-сервера. Если сервер
// не успевает остановиться штатно, вызывается принудительная остановка.
func stopGRPCServer(grpcSrv *grpc.Server) {
	if grpcSrv == nil {
		return
	}

	log.Println("Stopping gRPC server...")

	stopped := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		log.Println("gRPC server stopped")
	case <-time.After(shutdownTimeout):
		log.Println("gRPC graceful stop timed out, forcing stop")
		grpcSrv.Stop()
	}
}
