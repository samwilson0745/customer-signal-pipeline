// Package middleware holds cross-cutting HTTP wrappers shared by handlers.
package middleware

import "net/http"

// CORS allows the static dashboard (served from its own origin/port) to call
// this API directly from the browser with fetch(), including the
// Authorization header the JWT-protected routes require. The API has no
// cookie-based session, so a permissive origin doesn't expose credentials.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
