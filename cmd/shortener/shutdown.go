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

// servers группирует все серверы, управляемые run()
type servers struct {
	// http — основной HTTP-сервер. Обязателен.
	http *http.Server
	// pprof — вспомогательный отладочный сервер; может быть nil, тогда не запускается.
	pprof *http.Server
	// grpc — gRPC-сервер; может быть nil, тогда не запускается.
	grpc *grpc.Server
	// grpcAddr — адрес, на котором grpc.Serve начинает слушать. Игнорируется, если grpc == nil.
	grpcAddr string
}

// run запускает srv.http, вспомогательный srv.pprof (если не nil) и srv.grpc (если не nil),
// затем блокируется до получения сигнала завершения или ошибки запуска основного HTTP-
// или gRPC-сервера. После сигнала выполняет graceful shutdown всех серверов: ждет
// завершения активных запросов, затем вызывает onShutdown для сохранения несохраненных
// данных. onShutdown вызывается гарантированно — даже при ошибке shutdown.
//
// Ошибка запуска pprof-сервера не останавливает остальные серверы и не возвращается
// из run — это вспомогательный отладочный сервер, его недоступность не критична
// для работы сервиса; но при остановке он гасится вместе со всеми остальными.
func run(ctx context.Context, srv servers, cfg *config.Config, onShutdown func()) error {
	// Каналы для получения ошибок запуска серверов
	httpErrChan := make(chan error, 1)

	go func() {
		log.Printf("Server starting on %s (HTTPS: %v)", srv.http.Addr, cfg.EnableHTTPS)
		log.Printf("Result link direct to: %s", cfg.ResultURL)

		err := listenAndServe(srv.http, cfg)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			httpErrChan <- err
		} else {
			httpErrChan <- nil
		}
	}()

	if srv.pprof != nil {
		go func() {
			log.Printf("pprof server starting on %s", srv.pprof.Addr)

			if err := srv.pprof.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("pprof server error: %v", err)
			}
		}()
	}

	// grpcErrChan остается nil, если gRPC-сервер не сконфигурирован — получение
	// из nil-канала блокируется навсегда, поэтому select ниже просто игнорирует эту ветку.
	var grpcErrChan chan error
	if srv.grpc != nil {
		grpcErrChan = make(chan error, 1)

		go func() {
			lis, err := net.Listen("tcp", srv.grpcAddr)
			if err != nil {
				grpcErrChan <- err
				return
			}

			log.Printf("gRPC server starting on %s (TLS: %v)", srv.grpcAddr, cfg.EnableHTTPS)

			if err := srv.grpc.Serve(lis); err != nil {
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

		if err := stopHTTPServer(srv.http, "HTTP server"); err != nil {
			runErr = err
		}

		stopHTTPServer(srv.pprof, "pprof server") //nolint:errcheck // недоступность отладочного сервера не критична
		stopGRPCServer(srv.grpc)

	case err := <-httpErrChan:
		// HTTP-сервер завершился до получения сигнала (ошибка запуска)
		if err != nil {
			runErr = err
		}
		stopHTTPServer(srv.pprof, "pprof server") //nolint:errcheck // недоступность отладочного сервера не критична
		stopGRPCServer(srv.grpc)

	case err := <-grpcErrChan:
		// gRPC-сервер завершился до получения сигнала (ошибка запуска)
		if err != nil {
			runErr = err
		}

		stopHTTPServer(srv.http, "HTTP server")   //nolint:errcheck // ошибку остановки основного сервера уже некуда возвращать содержательно
		stopHTTPServer(srv.pprof, "pprof server") //nolint:errcheck // недоступность отладочного сервера не критична
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
