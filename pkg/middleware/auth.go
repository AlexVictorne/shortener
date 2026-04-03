package middleware

import (
	"context"
	"net/http"
	"shortener/pkg/auth"
)

type contextKey string

const UserIDKey contextKey = "userID"
const cookieName = "auth_token"

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

func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey).(string)
	return userID, ok && userID != ""
}

// UserIDKey returns the context key for user ID (for cross-package usage)
func UserIDKeyFunc() interface{} {
	return UserIDKey
}
