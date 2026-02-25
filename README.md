# nghttp

A lightweight Go library providing utilities for handling HTTP responses and middleware in a consistent manner.

## Installation

```sh
go get github.com/nickelghost/nghttp
```

## Features

### Middleware

#### Request ID (`UseRequestID`)

Reads a request ID from an incoming header (e.g. `X-Request-ID`). If not present, generates a new UUID. Stores the ID in the request context and reflects it back in the response header.

The `maxIDLen` parameter sets the maximum accepted byte length of an incoming request ID — values exceeding it are replaced with a fresh UUID to guard against log injection. Set it to `0` to disable the limit.

```go
handler = nghttp.UseRequestID(handler, nghttp.DefaultRequestIDHeader, 128)
```

#### Request Logging (`UseRequestLogging`)

Logs each request's method, path, status code, duration, and request ID using `log/slog` structured logging.

```go
handler = nghttp.UseRequestLogging(handler, nil)
```

#### CORS (`UseCORS`)

Adds `Access-Control-Allow-Origin`, `Access-Control-Allow-Headers`, and `Access-Control-Allow-Methods` headers. Automatically handles `OPTIONS` preflight requests.

```go
handler = nghttp.UseCORS(
    handler,
    []string{"https://example.com"},
    []string{"Content-Type", "Authorization"},
    []string{"GET", "POST", "PUT", "DELETE"},
    false, // set to true to send Access-Control-Allow-Credentials: true
    nil,
)
```

### Response Helpers

#### `Respond`

Encodes any value as JSON and writes it with the given status code. Automatically logs at the appropriate level (`Info`, `Warn`, or `Error`) based on the status code, including the request ID from context. If `res` is `nil`, a `{"message": "<status text>"}` body is sent instead.

```go
nghttp.Respond(w, r, http.StatusOK, nil, myData, nil)
```

#### `GetNotFoundHandler`

Returns an `http.HandlerFunc` that responds with `404 Not Found` using `RespondGeneric`.

```go
mux.NotFound(nghttp.GetNotFoundHandler(nil))
```

### Utilities

#### `GetRequestID`

Retrieves the request ID stored in a `context.Context` by `UseRequestID`.

```go
id := nghttp.GetRequestID(r.Context())
```

## Custom Log Arguments

Most functions accept an optional `getLogArgs func(ctx context.Context) []any` parameter. When provided, the returned key-value pairs are appended to every structured log entry, allowing you to inject per-request fields (e.g. authenticated user ID) from context.

```go
getLogArgs := func(ctx context.Context) []any {
    return []any{"userID", UserIDFromContext(ctx)}
}

handler = nghttp.UseRequestLogging(handler, getLogArgs)
```

## Requirements

- Go 1.24+
