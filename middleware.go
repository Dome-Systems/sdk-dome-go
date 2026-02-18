package dome

import (
	"net/http"
)

// Middleware wraps an http.Handler with Dome governance. Currently logs
// requests via the configured logger. Policy enforcement will be added
// in a future version.
func (c *Client) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.logger.Info("dome: request",
			"method", r.Method,
			"path", r.URL.Path,
		)
		next.ServeHTTP(w, r)
	})
}
