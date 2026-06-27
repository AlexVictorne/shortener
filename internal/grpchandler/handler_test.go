package grpchandler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"shortener/internal/pb"
	"shortener/internal/repository"
	"shortener/internal/service"
	"shortener/pkg/auth"
	"shortener/pkg/generator"
	"shortener/pkg/middleware"
)

// newTestServer создает Server с MemStorage и фиксированным секретом для тестов.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	stor := repository.NewMemStorage()
	gen := generator.NewGenerator(8)
	svc := service.NewTrimmerService(stor, gen, "http://localhost:8080/")
	return NewServer(svc, "test_secret")
}

// authContext создает контекст с валидным заголовком authorization в gRPC metadata.
func authContext(userID, secret string) context.Context {
	token := auth.Sign(userID, secret)
	md := metadata.Pairs("authorization", token)
	return metadata.NewIncomingContext(context.Background(), md)
}

// TestShortenURL_Success проверяет успешное сокращение валидного URL.
func TestShortenURL_Success(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	resp, err := srv.ShortenURL(ctx, &pb.URLShortenRequest{Url: "https://example.com"})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetResult())
	assert.Contains(t, resp.GetResult(), "http://localhost:8080/")
}

// TestShortenURL_Conflict проверяет, что повторное сокращение того же URL
// возвращает AlreadyExists и содержит существующий короткий URL.
func TestShortenURL_Conflict(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	first, err := srv.ShortenURL(ctx, &pb.URLShortenRequest{Url: "https://conflict.example.com"})
	require.NoError(t, err)

	_, err = srv.ShortenURL(ctx, &pb.URLShortenRequest{Url: "https://conflict.example.com"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.AlreadyExists, st.Code())
	// Убеждаемся, что первый ответ был успешным.
	assert.NotEmpty(t, first.GetResult())
}

// TestShortenURL_ValidationError проверяет, что невалидный URL
// возвращает статус InvalidArgument.
func TestShortenURL_ValidationError(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		name string
		url  string
	}{
		{"empty url", ""},
		{"no scheme", "example.com"},
		{"ftp scheme", "ftp://example.com"},
		{"no host", "https://"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := srv.ShortenURL(context.Background(), &pb.URLShortenRequest{Url: tt.url})
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.InvalidArgument, st.Code())
		})
	}
}

// TestShortenURL_WithAuth проверяет, что при валидном заголовке authorization
// URL привязывается к userID пользователя.
func TestShortenURL_WithAuth(t *testing.T) {
	srv := newTestServer(t)
	ctx := authContext("user123", "test_secret")

	resp, err := srv.ShortenURL(ctx, &pb.URLShortenRequest{Url: "https://auth-test.example.com"})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetResult())
}

// TestExpandURL_Success проверяет успешное раскрытие короткого идентификатора.
func TestExpandURL_Success(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	// Сначала сокращаем URL.
	shortenResp, err := srv.ShortenURL(ctx, &pb.URLShortenRequest{Url: "https://expand.example.com"})
	require.NoError(t, err)

	// Извлекаем короткий идентификатор из полного URL.
	shortURL := shortenResp.GetResult()
	id := shortURL[len("http://localhost:8080/"):]

	expandResp, err := srv.ExpandURL(ctx, &pb.URLExpandRequest{Id: id})
	require.NoError(t, err)
	assert.Equal(t, "https://expand.example.com", expandResp.GetResult())
}

// TestExpandURL_NotFound проверяет, что несуществующий идентификатор
// возвращает статус NotFound.
func TestExpandURL_NotFound(t *testing.T) {
	srv := newTestServer(t)

	_, err := srv.ExpandURL(context.Background(), &pb.URLExpandRequest{Id: "notexist"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

// TestListUserURLs_Success проверяет, что после сокращения URL с авторизацией
// ListUserURLs возвращает корректный список.
func TestListUserURLs_Success(t *testing.T) {
	srv := newTestServer(t)
	ctx := authContext("list-user", "test_secret")

	// Сокращаем два URL под одним пользователем.
	_, err := srv.ShortenURL(ctx, &pb.URLShortenRequest{Url: "https://list1.example.com"})
	require.NoError(t, err)
	_, err = srv.ShortenURL(ctx, &pb.URLShortenRequest{Url: "https://list2.example.com"})
	require.NoError(t, err)

	resp, err := srv.ListUserURLs(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	assert.Len(t, resp.GetUrl(), 2)
}

// TestListUserURLs_NoContent проверяет, что пользователь без URL получает
// пустой список (не ошибку).
func TestListUserURLs_NoContent(t *testing.T) {
	srv := newTestServer(t)
	ctx := authContext("empty-user", "test_secret")

	resp, err := srv.ListUserURLs(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	assert.Empty(t, resp.GetUrl())
}

// TestListUserURLs_Unauthenticated проверяет, что запрос без заголовка authorization
// возвращает статус Unauthenticated.
func TestListUserURLs_Unauthenticated(t *testing.T) {
	srv := newTestServer(t)

	_, err := srv.ListUserURLs(context.Background(), &emptypb.Empty{})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

// TestExtractUserID_ValidToken проверяет, что валидный токен корректно верифицируется.
func TestExtractUserID_ValidToken(t *testing.T) {
	secret := "test_secret"
	expectedID := "myuserid"
	token := auth.Sign(expectedID, secret)

	md := metadata.Pairs("authorization", token)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	gotID, _, ok := extractUserIDStrict(ctx, secret)
	require.True(t, ok)
	assert.Equal(t, expectedID, gotID)
}

// TestExtractUserID_InvalidToken проверяет, что невалидный токен не проходит верификацию.
func TestExtractUserID_InvalidToken(t *testing.T) {
	md := metadata.Pairs("authorization", "invalid:token:value")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, _, ok := extractUserIDStrict(ctx, "test_secret")
	assert.False(t, ok)
}

// TestExtractUserID_Missing проверяет, что отсутствие заголовка authorization
// приводит к генерации нового userID в extractUserID.
func TestExtractUserID_Missing(t *testing.T) {
	ctx := context.Background()
	userID, newCtx := extractUserID(ctx, "test_secret")
	assert.NotEmpty(t, userID)
	// Контекст обогащен новым userID.
	val := newCtx.Value(middleware.UserIDKey)
	assert.Equal(t, userID, val)
}
