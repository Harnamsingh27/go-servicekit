package observability

import (
	"context"
	"log/slog"
	"os"
)

type loggerKey struct{}

// NewLogger creates a JSON-mode slog.Logger writing to stdout at the given level.
func NewLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
	}))
}

// WithLogger attaches l to ctx so nested components can retrieve it.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// LoggerFromContext returns the logger stored in ctx. If none is present it
// returns the global slog default logger.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
