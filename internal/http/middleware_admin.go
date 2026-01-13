package httpapi

import (
	"net/http"
	"os"
	"strings"
)

func AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(os.Getenv("APP_ENV"), "dev") {
			next.ServeHTTP(w, r)
			return
		}

		adminKey := strings.TrimSpace(os.Getenv("ADMIN_KEY"))
		if adminKey == "" {
			http.Error(w, "admin access not configured", http.StatusUnauthorized)
			return
		}

		if strings.TrimSpace(r.Header.Get("X-Admin-Key")) != adminKey {
			http.Error(w, "invalid admin key", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
