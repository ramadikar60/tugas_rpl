package middleware

import (
	"context"
	"net/http"
	"strings"

	"notebook-app/auth"
)

type contextKey string

const (
	UserIDKey      contextKey = "user_id"
	AccessTokenKey contextKey = "access_token"
)

func Auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			token = strings.TrimPrefix(h, "Bearer ")
		} else if c, err := r.Cookie("access_token"); err == nil {
			token = c.Value
		}

		if token == "" {
			http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		userID, err := auth.VerifyAccessToken(token)
		if err != nil {
			http.Error(w, `{"error":"Invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, AccessTokenKey, token)
		next(w, r.WithContext(ctx))
	}
}

func GetUserID(r *http.Request) int64 {
	if id, ok := r.Context().Value(UserIDKey).(int64); ok {
		return id
	}
	return 0
}

func GetAccessToken(r *http.Request) string {
	if t, ok := r.Context().Value(AccessTokenKey).(string); ok {
		return t
	}
	return ""
}
