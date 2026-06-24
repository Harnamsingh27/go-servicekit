package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/harnamsingh/go-servicekit/httpx"
	"github.com/harnamsingh/go-servicekit/observability"
)

func main() {
	logger := observability.NewLogger(
		observability.WithLogLevel(slog.LevelDebug),
	)

	shutdown, err := observability.InitTracer("http-demo",
		observability.WithStdoutTracer(os.Stdout),
	)
	if err != nil {
		logger.Error("init tracer", slog.Any("err", err))
		os.Exit(1)
	}
	defer shutdown(context.Background()) //nolint:errcheck

	mws := httpx.DefaultMiddleware(logger, 10*time.Second)
	srv := httpx.NewServer(
		httpx.WithAddr(":8080"),
		httpx.WithServerMiddleware(mws...),
	)

	r := srv.Router()
	r.GET("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	v1 := r.Group("/v1")
	v1.GET("/hello", func(w http.ResponseWriter, req *http.Request) {
		tracer := observability.Tracer("http-demo")
		_, span := tracer.Start(req.Context(), "hello-handler")
		defer span.End()

		name := req.URL.Query().Get("name")
		if name == "" {
			name = "world"
		}
		fmt.Fprintf(w, "Hello, %s!\n", name)
	})

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("http-demo listening", slog.String("addr", ":8080"))
		if err := srv.ListenAndServe(); err != nil {
			logger.Error("server stopped", slog.Any("err", err))
		}
	}()

	<-stop
	logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown", slog.Any("err", err))
	}
}
