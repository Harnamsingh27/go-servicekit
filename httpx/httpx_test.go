package httpx_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/harnamsingh/go-servicekit/httpx"
	"github.com/harnamsingh/go-servicekit/observability"
)

func newLogger() *slog.Logger { return observability.NewLogger(observability.WithLogOutput(io.Discard)) }

// ---- PanicRecoveryMiddleware -----------------------------------------------

func TestPanicRecoveryMiddleware_Catches(t *testing.T) {
	h := httpx.PanicRecoveryMiddleware(newLogger())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestPanicRecoveryMiddleware_PassThrough(t *testing.T) {
	h := httpx.PanicRecoveryMiddleware(newLogger())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// ---- AccessLogMiddleware ---------------------------------------------------

func TestAccessLogMiddleware_Runs(t *testing.T) {
	var buf strings.Builder
	logger := observability.NewLogger(observability.WithLogOutput(&buf))
	h := httpx.AccessLogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/items", nil))
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
	if !strings.Contains(buf.String(), "http request") {
		t.Errorf("log missing 'http request'; got: %s", buf.String())
	}
}

// ---- TimeoutMiddleware -----------------------------------------------------

func TestTimeoutMiddleware_Timeout(t *testing.T) {
	h := httpx.TimeoutMiddleware(10 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/slow", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestTimeoutMiddleware_Fast(t *testing.T) {
	h := httpx.TimeoutMiddleware(time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fast", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// ---- DefaultMiddleware -----------------------------------------------------

func TestDefaultMiddleware_Chain(t *testing.T) {
	mws := httpx.DefaultMiddleware(newLogger(), time.Second)
	if len(mws) == 0 {
		t.Fatal("expected non-empty middleware chain")
	}
}

// ---- Router ----------------------------------------------------------------

func TestRouter_GET(t *testing.T) {
	r := httpx.NewRouter()
	called := false
	r.GET("/hello", func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hello", nil))
	if !called {
		t.Error("GET /hello handler not called")
	}
}

func TestRouter_POST(t *testing.T) {
	r := httpx.NewRouter()
	r.POST("/items", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/items", nil))
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
}

func TestRouter_Group(t *testing.T) {
	r := httpx.NewRouter()
	v1 := r.Group("/v1")
	v1.GET("/ping", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, "pong")
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/ping", nil))
	if body := rec.Body.String(); body != "pong" {
		t.Errorf("body = %q, want pong", body)
	}
}

func TestRouter_MultipleRoutes(t *testing.T) {
	r := httpx.NewRouter()
	r.GET("/a", func(w http.ResponseWriter, req *http.Request) { fmt.Fprint(w, "a") })
	r.GET("/b", func(w http.ResponseWriter, req *http.Request) { fmt.Fprint(w, "b") })

	for path, want := range map[string]string{"/a": "a", "/b": "b"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Body.String(); got != want {
			t.Errorf("GET %s = %q, want %q", path, got, want)
		}
	}
}

func TestRouter_HandleFunc(t *testing.T) {
	r := httpx.NewRouter()
	r.HandleFunc(http.MethodPut, "/thing", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/thing", nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}

// ---- Server ----------------------------------------------------------------

func TestNewServer_ServesRequests(t *testing.T) {
	logger := newLogger()
	mws := httpx.DefaultMiddleware(logger, 5*time.Second)
	srv := httpx.NewServer(
		httpx.WithAddr("127.0.0.1:0"),
		httpx.WithServerMiddleware(mws...),
	)
	srv.Router().GET("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	resp, err := http.Get(fmt.Sprintf("http://%s/health", ln.Addr()))
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
	<-errc
}

func TestNewServer_PanicRecovery(t *testing.T) {
	logger := newLogger()
	srv := httpx.NewServer(
		httpx.WithServerMiddleware(httpx.PanicRecoveryMiddleware(logger)),
	)
	srv.Router().GET("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve(ln) //nolint:errcheck

	resp, err := http.Get(fmt.Sprintf("http://%s/panic", ln.Addr()))
	if err != nil {
		t.Fatalf("GET /panic: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	srv.Shutdown(ctx) //nolint:errcheck
}

// ---- Client ----------------------------------------------------------------

func TestClient_Get_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello")
	}))
	defer ts.Close()

	c := httpx.NewClient(httpx.WithRetry(0, 0))
	resp, err := c.Get(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Errorf("body = %q, want hello", body)
	}
}

func TestClient_PropagatesRequestID(t *testing.T) {
	var gotID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get(observability.RequestIDHeader)
	}))
	defer ts.Close()

	ctx := observability.WithRequestID(context.Background(), "test-rid")
	c := httpx.NewClient(httpx.WithRetry(0, 0))
	resp, err := c.Get(ctx, ts.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if gotID != "test-rid" {
		t.Errorf("X-Request-ID = %q, want test-rid", gotID)
	}
}

func TestClient_RetriesOn5xx(t *testing.T) {
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := httpx.NewClient(httpx.WithRetry(3, time.Millisecond))
	resp, err := c.Get(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Get after retries: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("final status = %d, want 200", resp.StatusCode)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestClient_AllRetrysFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := httpx.NewClient(httpx.WithRetry(2, time.Millisecond))
	_, err := c.Get(context.Background(), ts.URL)
	if err == nil {
		t.Error("expected error after all retries fail")
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	c := httpx.NewClient(httpx.WithClientTimeout(time.Second))
	_, err := c.Get(ctx, ts.URL)
	if err == nil {
		t.Error("expected error from context cancellation")
	}
}
