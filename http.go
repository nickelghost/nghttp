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
)

// DefaultRequestIDHeader is the default header name used for the request ID.
const DefaultRequestIDHeader = "X-Request-ID"

type contextKey string

const requestIDKey = contextKey("requestID")

// GetRequestID returns the trace ID from the context if it exists.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}

	return ""
}

// GenericResponse is a simple structure used for responses without any
// additional data, instead only containing a message. It is mostly used for
// error responses.
type GenericResponse struct {
	Message string `json:"message"`
}

// Respond is a utility function to send a JSON response with a specific HTTP
// status code. It logs the request ID and trace path, and handles different
// levels of errors based on the status code. If res is nil, a generic
// {"message": "<status text>"} body is sent instead of null.
func Respond(
	w http.ResponseWriter,
	r *http.Request,
	code int,
	err error,
	res any,
	getLogArgs func(ctx context.Context) []any,
) {
	ctx := r.Context()
	requestID := GetRequestID(ctx)
	statusText := http.StatusText(code)
	logger := slog.With("requestID", requestID)

	if getLogArgs != nil {
		if logArgs := getLogArgs(ctx); logArgs != nil {
			logger = logger.With(logArgs...)
		}
	}

	switch {
	case code >= http.StatusInternalServerError:
		logger.Error(statusText, "err", err)
	case code >= http.StatusBadRequest:
		logger.Warn(statusText, "err", err)
	default:
		logger.Info(statusText)
	}

	if res == nil {
		res = GenericResponse{Message: http.StatusText(code)}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	if err := json.NewEncoder(w).Encode(res); err != nil {
		logger.Error("failed to encode response", "err", err)

		return
	}
}

// GetNotFoundHandler returns a handler function that responds with a 404 Not Found
// status code. It uses the Respond function to send a generic response
// with the appropriate status message.
func GetNotFoundHandler(getLogArgs func(ctx context.Context) []any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		Respond(w, r, http.StatusNotFound, nil, nil, getLogArgs)
	}
}

// UseRequestID is a middleware that checks for the presence of a request ID in
// the request headers. If it is not present, it generates a new UUID and adds
// it to the request context. This allows downstream handlers to access the
// request ID for logging or tracing purposes.
//
// maxIDLen sets the maximum accepted byte length of an incoming request ID. If
// the header value exceeds this limit it is discarded and a fresh UUID is used
// instead, guarding against log injection. Set maxIDLen to 0 to disable the
// limit entirely.
func UseRequestID(next http.Handler, headerName string, maxIDLen int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerName)
		if id == "" || (maxIDLen > 0 && len(id) > maxIDLen) {
			id = uuid.NewString()
		}

		w.Header().Set(headerName, id)

		ctx := context.WithValue(r.Context(), requestIDKey, id)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// loggingResponseWriter wraps http.ResponseWriter to capture the status code
// written by the handler, so it can be included in request logs.
type loggingResponseWriter struct {
	http.ResponseWriter

	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// UseRequestLogging is a middleware that logs the details of each HTTP request
// including the method, path, status code, duration, and request ID. It uses the slog
// package for structured logging. The duration is calculated from the start of the
// request to the completion of the response.
func UseRequestLogging(next http.Handler, getLogArgs func(ctx context.Context) []any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lrw, r)

		ctx := r.Context()
		requestID := GetRequestID(ctx)

		logger := slog.With(
			"method", r.Method,
			"path", r.URL.Path,
			"status", lrw.statusCode,
			"duration", time.Since(start),
			"requestID", requestID,
		)

		if getLogArgs != nil {
			if logArgs := getLogArgs(ctx); logArgs != nil {
				logger = logger.With(logArgs...)
			}
		}

		logger.Info("Request completed")
	})
}

// UseCORS is a middleware that adds CORS headers to the HTTP response. It checks
// the request's Origin header against a list of allowed origins and sets the
// Access-Control-Allow-Origin header accordingly. It also sets the allowed
// headers and methods for CORS requests. If the request method is OPTIONS, it
// responds with a 200 OK status and the appropriate CORS headers without
// processing the request further. For other methods, it calls the next handler
// in the chain.
//
// When allowCredentials is true, the Access-Control-Allow-Credentials header is
// set to "true", enabling credentialed requests (e.g. cookies, Authorization
// headers). Note that browsers will reject credentialed responses when
// Access-Control-Allow-Origin is "*"; use explicit origins instead.
func UseCORS(
	next http.Handler,
	allowedOrigins []string,
	allowedHeaders []string,
	allowedMethods []string,
	allowCredentials bool,
	getLogArgs func(ctx context.Context) []any,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		for _, allowedOrigin := range allowedOrigins {
			if allowedOrigin == origin || allowedOrigin == "*" {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)

				break
			}
		}

		w.Header().Set("Access-Control-Allow-Headers", strings.Join(allowedHeaders, ", "))
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(allowedMethods, ", "))

		if allowCredentials {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == http.MethodOptions {
			Respond(w, r, http.StatusOK, nil, nil, getLogArgs)

			return
		}

		next.ServeHTTP(w, r)
	})
}
