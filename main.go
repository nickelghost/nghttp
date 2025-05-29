// Package nghttp provides utilities for handling HTTP responses in a consistent
// manner across the application. It includes functions for sending JSON
// responses, logging request IDs, and handling different HTTP status codes.
package nghttp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nickelghost/ngtel"
)

// DefaultRequestIDHeader is the default header name used for the request ID.
const DefaultRequestIDHeader = "X-Request-ID"

// RequestIDKey is the key used to store the request ID in the context.
const RequestIDKey = "requestID"

// GenericResponse is a simple structure used for responses without any
// additional data, instead only containing a message. It is mostly used for
// error responses.
type GenericResponse struct {
	Message string `json:"message"`
}

// Respond is a utility function to send a JSON response with a specific HTTP
// status code. It logs the request ID and trace path, and handles different
// levels of errors based on the status code.
func Respond(w http.ResponseWriter, r *http.Request, code int, err error, res any) {
	ctx := r.Context()
	requestID, _ := ctx.Value(RequestIDKey).(string)
	statusText := http.StatusText(code)
	logger := slog.With("requestID", requestID, "trace", ngtel.GetCloudTracePath(ctx))

	switch {
	case code >= http.StatusInternalServerError:
		logger.Error(statusText, "err", err)
	case code >= http.StatusBadRequest:
		logger.Warn(statusText, "err", err)
	default:
		logger.Info(statusText)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(code)

	if err := json.NewEncoder(w).Encode(res); err != nil {
		logger.Error("failed to encode response", "err", err)
		w.WriteHeader(http.StatusInternalServerError)

		return
	}
}

// RespondGeneric is a convenience function to respond with a generic message
// based on the provided HTTP status code. It uses the http.StatusText for the
// message and does not include any additional data in the response body.
func RespondGeneric(w http.ResponseWriter, r *http.Request, code int, err error) {
	res := GenericResponse{Message: http.StatusText(code)}
	Respond(w, r, code, err, res)
}

// NotFoundHandler is a handler function that responds with a 404 Not Found
// status code. It uses the RespondGeneric function to send a generic response
// with the appropriate status message.
func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	RespondGeneric(w, r, http.StatusNotFound, nil)
}

// UseRequestID is a middleware that checks for the presence of a request ID in
// the request headers. If it is not present, it generates a new UUID and adds
// it to the request context. This allows downstream handlers to access the
// request ID for logging or tracing purposes.
func UseRequestID(headerName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerName)
		if id == "" {
			id = uuid.NewString()
		}

		ctx := context.WithValue(r.Context(), RequestIDKey, id) //nolint:revive,staticcheck

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UseRequestLogging is a middleware that logs the details of each HTTP request
// including the method, path, duration, and request ID. It uses the slog
// package for structured logging and includes the trace path from ngtel for
// better observability. The duration is calculated from the start of the
// request to the completion of the response.
func UseRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx := r.Context()

		next.ServeHTTP(w, r)

		requestID, _ := ctx.Value(RequestIDKey).(string)

		slog.Info(
			"Request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
			"requestID", requestID,
			"trace", ngtel.GetCloudTracePath(ctx),
		)
	})
}

// UseCORS is a middleware that adds CORS headers to the HTTP response. It checks
// the request's Origin header against a list of allowed origins and sets the
// Access-Control-Allow-Origin header accordingly. It also sets the allowed
// headers and methods for CORS requests. If the request method is OPTIONS, it
// responds with a 200 OK status and the appropriate CORS headers without
// processing the request further. For other methods, it calls the next handler
// in the chain.
func UseCORS(
	allowedOrigins []string, allowedHeaders []string, allowedMethods []string, next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		for _, allowedOrigin := range allowedOrigins {
			if allowedOrigin == origin {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			}
		}

		w.Header().Set("Access-Control-Allow-Headers", strings.Join(allowedHeaders, ", "))
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(allowedMethods, ", "))

		if r.Method == http.MethodOptions {
			RespondGeneric(w, r, http.StatusOK, nil)

			return
		}

		next.ServeHTTP(w, r)
	})
}
