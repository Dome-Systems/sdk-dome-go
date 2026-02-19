package dome

import (
	"net/http"
	"strings"
)

// Middleware wraps an http.Handler with Dome governance. Each incoming request
// is evaluated against the Cedar policy bundle. If denied, the request
// receives a 403 Forbidden response with the denial reason.
//
// If no policies are loaded, all requests are allowed (fail-open for v0.4.0).
func (c *Client) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := httpMethodToAction(r.Method)
		resource := strings.TrimPrefix(r.URL.Path, "/")

		decision, err := c.Check(r.Context(), CheckRequest{
			Action:   action,
			Resource: resource,
		})
		if err != nil {
			c.logger.Error("dome: policy check error", "error", err)
			// Fail-open on error.
			next.ServeHTTP(w, r)
			return
		}

		if !decision.Allowed {
			c.logger.Warn("dome: request denied",
				"method", r.Method,
				"path", r.URL.Path,
				"reason", decision.Reason,
			)
			http.Error(w, "Forbidden: "+decision.Reason, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func httpMethodToAction(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead:
		return "read"
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return strings.ToLower(method)
	}
}
