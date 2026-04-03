package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"shortener/pkg/auth"
	"testing"
)

func TestAuthMiddleware_NewUser(t *testing.T) {
	secret := "testsecret"
	var gotUserID string
	h := AuthMiddleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok || userID == "" {
			t.Error("userID not set in context for new user")
		}
		gotUserID = userID
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rw := httptest.NewRecorder()

	h.ServeHTTP(rw, req)

	resp := rw.Result()
	cookie := resp.Cookies()
	if len(cookie) == 0 || cookie[0].Name != "auth_token" {
		t.Error("auth_token cookie not set for new user")
	}
	if gotUserID == "" {
		t.Error("userID not set for new user")
	}
}

func TestAuthMiddleware_ExistingUser(t *testing.T) {
	secret := "testsecret"
	userID, _ := auth.NewUserID()
	cookieVal := auth.Sign(userID, secret)
	var called bool
	h := AuthMiddleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxUserID, ok := UserIDFromContext(r.Context())
		if !ok || ctxUserID != userID {
			t.Errorf("expected userID %q in context, got %q", userID, ctxUserID)
		}
		called = true
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: cookieVal})
	rw := httptest.NewRecorder()

	h.ServeHTTP(rw, req)

	if !called {
		t.Error("handler not called for existing user")
	}
}

func TestUserIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	userID, ok := UserIDFromContext(ctx)
	if ok || userID != "" {
		t.Error("expected empty userID and ok=false for empty context")
	}
}
