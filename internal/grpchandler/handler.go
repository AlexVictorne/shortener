// Package grpchandler реализует gRPC-слой сервиса сокращения ссылок.
// Каждый метод является тонким фасадом над TrimmerService — вся бизнес-логика
// остается в пакете service, а grpchandler лишь транслирует gRPC-вызовы
// в вызовы сервиса и маппирует ошибки на gRPC-коды статусов.
//
// Аутентификация: клиент передает заголовок "authorization" в gRPC metadata
// в том же формате, что и HTTP-кука "auth_token": "userID:HMAC-SHA256".
// Если заголовок отсутствует или подпись невалидна, сервер генерирует новый userID
// (аналогично поведению HTTP AuthMiddleware).
//
// Аудит: успешные ShortenURL и ExpandURL отправляют события "shorten"/"follow"
// в audit.Auditor, переданный в NewServer — симметрично HTTP Handler.
package grpchandler

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"shortener/internal/pb"
	"shortener/internal/service"
	"shortener/pkg/audit"
	"shortener/pkg/auth"
	"shortener/pkg/middleware"
)

// Server реализует pb.ShortenerServiceServer, делегируя вызовы TrimmerService.
// Создавайте экземпляры через NewServer; не конструируйте структуру напрямую.
type Server struct {
	pb.UnimplementedShortenerServiceServer

	// svc — сервисный слой с бизнес-логикой (валидация, хранение, генерация).
	svc *service.TrimmerService
	// authSecret — секрет для верификации HMAC-подписи токена авторизации.
	authSecret string
	// auditor — приёмник аудит-событий; NoopAuditor, если явно не передан,
	// симметрично поведению HTTP Handler.
	auditor audit.Auditor
}

// NewServer создает Server с переданным TrimmerService и секретом аутентификации.
// auditor может быть nil — в этом случае используется audit.NoopAuditor{},
// как и в HTTP Handler, если опция WithAuditor не была указана.
func NewServer(svc *service.TrimmerService, authSecret string, auditor audit.Auditor) *Server {
	if auditor == nil {
		auditor = audit.NoopAuditor{}
	}
	return &Server{svc: svc, authSecret: authSecret, auditor: auditor}
}

// emitAudit отправляет аудит-событие, симметрично Handler.emitAudit в HTTP-слое.
// Ошибка эмита только логируется — аудит не должен блокировать основной RPC-поток.
func (s *Server) emitAudit(ctx context.Context, action, userID, url string) {
	if err := s.auditor.Emit(ctx, audit.Event{
		TS:     time.Now().Unix(),
		Action: action,
		UserID: userID,
		URL:    url,
	}); err != nil {
		log.Warn().Err(err).Msg("audit emit failed")
	}
}

// ShortenURL обрабатывает запрос на сокращение URL.
// Читает userID из metadata (заголовок "authorization"), кладет в контекст
// и делегирует вызов service.TrimURL.
//
// Коды ответов:
//   - OK: короткий URL в поле result
//   - AlreadyExists: URL уже существует (result содержит существующий короткий URL)
//   - InvalidArgument: URL не прошел валидацию
//   - Internal: внутренняя ошибка хранилища
func (s *Server) ShortenURL(ctx context.Context, req *pb.URLShortenRequest) (*pb.URLShortenResponse, error) {
	// Извлекаем userID из metadata и обогащаем контекст.
	userID, ctx := extractUserID(ctx, s.authSecret)

	shortURL, err := s.svc.TrimURL(ctx, req.GetUrl())
	if err != nil {
		if errors.Is(err, service.ErrConflict) {
			// Возвращаем существующий короткий URL вместе со статусом AlreadyExists.
			// Аудит не отправляется — URL уже был зафиксирован при первом сокращении,
			// симметрично поведению HTTP ShortenURLHandler.
			return &pb.URLShortenResponse{Result: shortURL}, status.Errorf(codes.AlreadyExists, "URL already exists: %s", shortURL)
		}
		return nil, mapServiceError("shorten_url", err)
	}
	s.emitAudit(ctx, "shorten", userID, req.GetUrl())
	return &pb.URLShortenResponse{Result: shortURL}, nil
}

// ExpandURL обрабатывает запрос на раскрытие короткого идентификатора.
// Делегирует вызов service.GetOriginalURL.
//
// Коды ответов:
//   - OK: оригинальный URL в поле result
//   - NotFound: идентификатор не найден или URL был удален
//   - Internal: внутренняя ошибка хранилища
func (s *Server) ExpandURL(ctx context.Context, req *pb.URLExpandRequest) (*pb.URLExpandResponse, error) {
	// Извлекаем userID из metadata для аудит-события — симметрично тому, как HTTP
	// RedirectHandler берет userID, назначенный AuthMiddleware, перед вызовом emitAudit.
	userID, ctx := extractUserID(ctx, s.authSecret)

	originalURL, err := s.svc.GetOriginalURL(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, service.ErrURLDeleted) {
			return nil, status.Error(codes.NotFound, "URL has been deleted")
		}
		return nil, status.Error(codes.NotFound, "URL not found")
	}
	s.emitAudit(ctx, "follow", userID, originalURL)
	return &pb.URLExpandResponse{Result: originalURL}, nil
}

// ListUserURLs возвращает все URL, сохраненные текущим пользователем.
// UserID извлекается из metadata (заголовок "authorization").
// Если авторизация отсутствует или невалидна — возвращает пустой список (аналог 204 в HTTP).
//
// Коды ответов:
//   - OK: список URLData в поле url (может быть пустым)
//   - Unauthenticated: заголовок authorization отсутствует
//   - Internal: внутренняя ошибка хранилища
func (s *Server) ListUserURLs(ctx context.Context, _ *emptypb.Empty) (*pb.UserURLsResponse, error) {
	// Проверяем наличие заголовка authorization: для ListUserURLs он обязателен.
	userID, ctx, ok := extractUserIDStrict(ctx, s.authSecret)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authorization required")
	}
	_ = ctx // контекст обогащен userID, но метод сервиса принимает userID явно

	urls, err := s.svc.GetURLsByUser(ctx, userID)
	if err != nil {
		if errors.Is(err, service.ErrNoContent) {
			// Нет URL — возвращаем пустой список (не ошибка).
			return &pb.UserURLsResponse{}, nil
		}
		log.Error().Err(err).Msg("list_user_urls: storage error")
		return nil, status.Error(codes.Internal, "internal error")
	}

	// Преобразуем model.UserURLResponse в pb.URLData.
	data := make([]*pb.URLData, 0, len(urls))
	for _, u := range urls {
		data = append(data, &pb.URLData{
			ShortUrl:    u.ShortURL,
			OriginalUrl: u.OriginalURL,
		})
	}
	return &pb.UserURLsResponse{Url: data}, nil
}

// extractUserID читает заголовок "authorization" из входящих gRPC metadata.
// Формат значения: "userID:HMAC-SHA256" — совпадает с HTTP-кукой auth_token.
// Если заголовок отсутствует или HMAC-подпись невалидна, генерирует новый userID
// (поведение аналогично HTTP AuthMiddleware: каждый анонимный клиент получает ID).
// Возвращает userID и обогащенный контекст.
func extractUserID(ctx context.Context, secret string) (string, context.Context) {
	md, _ := metadata.FromIncomingContext(ctx)
	if vals := md.Get("authorization"); len(vals) > 0 {
		if userID, ok := auth.Verify(vals[0], secret); ok {
			ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
			return userID, ctx
		}
	}
	// Заголовок отсутствует или подпись невалидна — генерируем временный userID.
	userID, _ := auth.NewUserID()
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	return userID, ctx
}

// extractUserIDStrict читает и верифицирует заголовок "authorization" из metadata.
// В отличие от extractUserID, не генерирует новый userID при ошибке верификации.
// Возвращает (userID, ctx, true) при успехе или ("", ctx, false) если авторизация отсутствует/невалидна.
func extractUserIDStrict(ctx context.Context, secret string) (string, context.Context, bool) {
	md, _ := metadata.FromIncomingContext(ctx)
	if vals := md.Get("authorization"); len(vals) > 0 {
		if userID, ok := auth.Verify(vals[0], secret); ok {
			ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
			return userID, ctx, true
		}
	}
	return "", ctx, false
}

// mapServiceError преобразует ошибки пакета service в gRPC-статусы. op — имя
// вызывающего RPC-метода, используется только для логирования непредвиденных
// (Internal) ошибок — аналогично тому, как HTTP-слой логирует ошибки хранилища
// через zerolog перед ответом клиенту.
func mapServiceError(op string, err error) error {
	switch {
	case errors.Is(err, service.ErrURLEmpty),
		errors.Is(err, service.ErrURLTooLong),
		errors.Is(err, service.ErrURLInvalidFormat),
		errors.Is(err, service.ErrURLInvalidScheme),
		errors.Is(err, service.ErrURLNoHost):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrURLDeleted):
		return status.Error(codes.NotFound, "URL has been deleted")
	case errors.Is(err, service.ErrNoContent):
		return status.Error(codes.NotFound, "no URLs found")
	default:
		log.Error().Err(err).Str("op", op).Msg("grpc: internal service error")
		return status.Error(codes.Internal, "internal error")
	}
}
