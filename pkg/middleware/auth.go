// Package middleware предоставляет HTTP-middleware для аутентификации, сжатия и логирования.
package middleware

import (
	"context"
	"net/http"
	"shortener/pkg/auth"
)

type contextKey string

const UserIDKey contextKey = "userID"
const cookieName = "auth_token"

// AuthMiddleware — middleware cookie-аутентификации.
// Проверяет подпись куки "auth_token" с помощью HMAC-SHA256 и secret.
// Если кука отсутствует или невалидна, генерирует новый userID, подписывает и устанавливает куку.
// UserID всегда доступен в контексте запроса через UserIDFromContext.
func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieName)
			var userID string
			ok := false
			if err == nil {
				userID, ok = auth.Verify(cookie.Value, secret)
			}
			if !ok {
				userID, _ = auth.NewUserID()
				value := auth.Sign(userID, secret)
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    value,
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
			}
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext извлекает userID из контекста запроса, установленного AuthMiddleware.
// Возвращает ("", false), если userID отсутствует или пуст.
func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey).(string)
	return userID, ok && userID != ""
}

// UserIDKeyFunc возвращает ключ контекста для userID для использования в других пакетах.
func UserIDKeyFunc() interface{} {
	return UserIDKey
}
