package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func loggingMiddleware(logger Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		logger.InfoContext(r.Context(), "http request", slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.Int("status", wrapped.status), slog.Duration("duration", time.Since(started)))
	})
}

func recoveryMiddleware(logger Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func(ctx context.Context) {
			if recover() != nil {
				logger.ErrorContext(ctx, "panic recovered")
				if wrapped, ok := w.(*statusWriter); ok && wrapped.wrote {
					return
				}
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}(r.Context())
		next.ServeHTTP(w, r)
	})
}

func chain(opts Options, next http.Handler) http.Handler {
	return loggingMiddleware(opts.Logger, recoveryMiddleware(opts.Logger, authMiddleware(opts.AuthToken, next)))
}
