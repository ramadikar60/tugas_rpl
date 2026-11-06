package middleware

import (
	"net/http"

	"notebook-app/auth"
)

func CSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next(w, r)
			return
		}

		session := GetAccessToken(r)
		if session == "" {
			http.Error(w, `{"error":"No session"}`, http.StatusUnauthorized)
			return
		}

		token := r.Header.Get("X-CSRF-Token")
		if token == "" {
			http.Error(w, `{"error":"Missing CSRF token"}`, http.StatusForbidden)
			return
		}

		if !auth.VerifyCSRFToken(session, token) {
			http.Error(w, `{"error":"Invalid CSRF token"}`, http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

func Chain(h http.HandlerFunc, mws ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
