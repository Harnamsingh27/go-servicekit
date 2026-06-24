package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gjwt "github.com/golang-jwt/jwt/v5"
	"github.com/harnamsingh/go-servicekit/auth"
	"github.com/harnamsingh/go-servicekit/errors"
	"google.golang.org/grpc/metadata"
)

var testSecret = []byte("super-secret-key")

func buildToken(t *testing.T, claims gjwt.MapClaims) string {
	t.Helper()
	tok := gjwt.NewWithClaims(gjwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(testSecret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

func validClaims() gjwt.MapClaims {
	return gjwt.MapClaims{
		"sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
}

// ---- Auth error mapping -----------------------------------------------

func TestAuthErrors_MapToHTTP(t *testing.T) {
	cases := []struct {
		err  *errors.AppError
		want int
	}{
		{auth.ErrMissingToken, http.StatusUnauthorized},
		{auth.ErrExpiredToken, http.StatusUnauthorized},
		{auth.ErrInvalidSignature, http.StatusUnauthorized},
		{auth.ErrInvalidToken, http.StatusUnauthorized},
		{auth.ErrMissingAPIKey, http.StatusUnauthorized},
		{auth.ErrInvalidAPIKey, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		got := errors.ToHTTPStatus(tc.err)
		if got != tc.want {
			t.Errorf("ToHTTPStatus(%s) = %d, want %d", tc.err.Code, got, tc.want)
		}
	}
}

// ---- HMACVerifier -------------------------------------------------------

func TestHMACVerifier_ValidToken(t *testing.T) {
	v := auth.NewHMACVerifier(testSecret)
	tok := buildToken(t, validClaims())
	claims, err := v.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims == nil {
		t.Fatal("expected non-nil claims")
	}
}

func TestHMACVerifier_ExpiredToken(t *testing.T) {
	v := auth.NewHMACVerifier(testSecret)
	tok := buildToken(t, gjwt.MapClaims{
		"sub": "user-1",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	_, err := v.Verify(tok)
	if err != auth.ErrExpiredToken {
		t.Errorf("got %v, want ErrExpiredToken", err)
	}
}

func TestHMACVerifier_WrongSecret(t *testing.T) {
	v := auth.NewHMACVerifier([]byte("wrong-secret"))
	tok := buildToken(t, validClaims())
	_, err := v.Verify(tok)
	if err != auth.ErrInvalidSignature {
		t.Errorf("got %v, want ErrInvalidSignature", err)
	}
}

func TestHMACVerifier_MalformedToken(t *testing.T) {
	v := auth.NewHMACVerifier(testSecret)
	_, err := v.Verify("not.a.jwt")
	if err == nil {
		t.Error("expected error for malformed token")
	}
}

func TestHMACVerifier_WithIssuer_Valid(t *testing.T) {
	v := auth.NewHMACVerifier(testSecret, auth.WithIssuer("my-issuer"))
	c := validClaims()
	c["iss"] = "my-issuer"
	tok := buildToken(t, c)
	_, err := v.Verify(tok)
	if err != nil {
		t.Fatalf("valid issuer: %v", err)
	}
}

func TestHMACVerifier_WithIssuer_Wrong(t *testing.T) {
	v := auth.NewHMACVerifier(testSecret, auth.WithIssuer("expected"))
	c := validClaims()
	c["iss"] = "wrong"
	tok := buildToken(t, c)
	_, err := v.Verify(tok)
	if err == nil {
		t.Error("expected error for wrong issuer")
	}
}

func TestHMACVerifier_WithAudience_Valid(t *testing.T) {
	v := auth.NewHMACVerifier(testSecret, auth.WithAudience("my-service"))
	c := validClaims()
	c["aud"] = "my-service"
	tok := buildToken(t, c)
	_, err := v.Verify(tok)
	if err != nil {
		t.Fatalf("valid audience: %v", err)
	}
}

// ---- JWTMiddleware HTTP --------------------------------------------------

func jwtHandler(v auth.Verifier) http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := auth.ClaimsFromContext(r.Context())
		if !ok || c == nil {
			http.Error(w, "no claims", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return auth.JWTMiddleware(v)(inner)
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
	h := jwtHandler(auth.NewHMACVerifier(testSecret))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+buildToken(t, validClaims()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestJWTMiddleware_MissingHeader(t *testing.T) {
	h := jwtHandler(auth.NewHMACVerifier(testSecret))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestJWTMiddleware_ExpiredToken(t *testing.T) {
	h := jwtHandler(auth.NewHMACVerifier(testSecret))
	tok := buildToken(t, gjwt.MapClaims{
		"sub": "user",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestJWTMiddleware_MalformedHeader(t *testing.T) {
	h := jwtHandler(auth.NewHMACVerifier(testSecret))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "NotBearer token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// ---- JWTUnaryInterceptor gRPC -------------------------------------------

func TestJWTUnaryInterceptor_ValidToken(t *testing.T) {
	interceptor := auth.JWTUnaryInterceptor(auth.NewHMACVerifier(testSecret))
	tok := buildToken(t, validClaims())
	md := metadata.Pairs("authorization", "Bearer "+tok)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	called := false
	_, err := interceptor(ctx, nil, nil, func(ctx context.Context, req any) (any, error) {
		called = true
		c, ok := auth.ClaimsFromContext(ctx)
		if !ok || c == nil {
			t.Error("claims missing from gRPC context")
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
}

func TestJWTUnaryInterceptor_MissingMetadata(t *testing.T) {
	interceptor := auth.JWTUnaryInterceptor(auth.NewHMACVerifier(testSecret))
	_, err := interceptor(context.Background(), nil, nil, func(ctx context.Context, req any) (any, error) {
		return nil, nil
	})
	if err == nil {
		t.Error("expected error for missing metadata")
	}
}

func TestJWTUnaryInterceptor_ExpiredToken(t *testing.T) {
	interceptor := auth.JWTUnaryInterceptor(auth.NewHMACVerifier(testSecret))
	tok := buildToken(t, gjwt.MapClaims{
		"sub": "user",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	md := metadata.Pairs("authorization", "Bearer "+tok)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := interceptor(ctx, nil, nil, func(ctx context.Context, req any) (any, error) {
		return nil, nil
	})
	if err == nil {
		t.Error("expected error for expired token in gRPC")
	}
}

// ---- API key middleware --------------------------------------------------

func TestMemoryKeyStore_ValidKey(t *testing.T) {
	store := auth.NewMemoryKeyStore("key-1", "key-2")
	ok, err := store.Valid(context.Background(), "key-1")
	if err != nil || !ok {
		t.Errorf("Valid(key-1) = %v, %v; want true, nil", ok, err)
	}
}

func TestMemoryKeyStore_InvalidKey(t *testing.T) {
	store := auth.NewMemoryKeyStore("key-1")
	ok, _ := store.Valid(context.Background(), "key-999")
	if ok {
		t.Error("expected key-999 to be invalid")
	}
}

func TestMemoryKeyStore_AddRemove(t *testing.T) {
	store := auth.NewMemoryKeyStore()
	store.AddKey("dynamic")
	ok, _ := store.Valid(context.Background(), "dynamic")
	if !ok {
		t.Error("added key should be valid")
	}
	store.RemoveKey("dynamic")
	ok, _ = store.Valid(context.Background(), "dynamic")
	if ok {
		t.Error("removed key should be invalid")
	}
}

func TestAPIKeyMiddleware_ValidKey(t *testing.T) {
	store := auth.NewMemoryKeyStore("secret-key")
	mw := auth.APIKeyMiddleware(store)
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(auth.APIKeyHeader, "secret-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !called {
		t.Errorf("status = %d called = %v; want 200 true", rec.Code, called)
	}
}

func TestAPIKeyMiddleware_InvalidKey(t *testing.T) {
	mw := auth.APIKeyMiddleware(auth.NewMemoryKeyStore("secret-key"))
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(auth.APIKeyHeader, "wrong-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAPIKeyMiddleware_MissingHeader(t *testing.T) {
	mw := auth.APIKeyMiddleware(auth.NewMemoryKeyStore("secret-key"))
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestClaimsFromContext_Empty(t *testing.T) {
	c, ok := auth.ClaimsFromContext(context.Background())
	if ok || c != nil {
		t.Error("expected no claims in bare context")
	}
}
