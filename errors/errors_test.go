package errors_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/harnamsingh/go-servicekit/errors"

	"google.golang.org/grpc/codes"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *errors.AppError
		want string
	}{
		{
			name: "without cause",
			err:  errors.New(errors.CodeNotFound, "user not found"),
			want: "[NOT_FOUND] user not found",
		},
		{
			name: "with cause",
			err:  errors.Wrap(errors.CodeInternal, "db failed", fmt.Errorf("conn refused")),
			want: "[INTERNAL] db failed: conn refused",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewf(t *testing.T) {
	err := errors.Newf(errors.CodeInvalidArgument, "field %q is required", "email")
	want := "[INVALID_ARGUMENT] field \"email\" is required"
	if err.Error() != want {
		t.Errorf("Newf() = %q, want %q", err.Error(), want)
	}
}

func TestAppError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("original")
	err := errors.Wrap(errors.CodeInternal, "wrapped", cause)
	if err.Unwrap() != cause {
		t.Error("Unwrap() did not return the original cause")
	}
}

func TestToHTTPStatus(t *testing.T) {
	tests := []struct {
		code errors.ErrorCode
		want int
	}{
		{errors.CodeNotFound, http.StatusNotFound},
		{errors.CodeUnauthorized, http.StatusUnauthorized},
		{errors.CodeForbidden, http.StatusForbidden},
		{errors.CodeInvalidArgument, http.StatusBadRequest},
		{errors.CodeAlreadyExists, http.StatusConflict},
		{errors.CodeDeadlineExceeded, http.StatusGatewayTimeout},
		{errors.CodeUnavailable, http.StatusServiceUnavailable},
		{errors.CodeInternal, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		err := errors.New(tt.code, "test error")
		if got := errors.ToHTTPStatus(err); got != tt.want {
			t.Errorf("ToHTTPStatus(%s) = %d, want %d", tt.code, got, tt.want)
		}
	}
}

func TestToHTTPStatus_NonAppError(t *testing.T) {
	err := fmt.Errorf("plain error")
	if got := errors.ToHTTPStatus(err); got != http.StatusInternalServerError {
		t.Errorf("ToHTTPStatus(plain) = %d, want 500", got)
	}
}

func TestToGRPCStatus(t *testing.T) {
	tests := []struct {
		code errors.ErrorCode
		want codes.Code
	}{
		{errors.CodeNotFound, codes.NotFound},
		{errors.CodeUnauthorized, codes.PermissionDenied},
		{errors.CodeForbidden, codes.PermissionDenied},
		{errors.CodeInvalidArgument, codes.InvalidArgument},
		{errors.CodeAlreadyExists, codes.AlreadyExists},
		{errors.CodeDeadlineExceeded, codes.DeadlineExceeded},
		{errors.CodeUnavailable, codes.Unavailable},
		{errors.CodeInternal, codes.Internal},
	}
	for _, tt := range tests {
		err := errors.New(tt.code, "test")
		st := errors.ToGRPCStatus(err)
		if st.Code() != tt.want {
			t.Errorf("ToGRPCStatus(%s).Code() = %v, want %v", tt.code, st.Code(), tt.want)
		}
	}
}

func TestToGRPCStatus_NonAppError(t *testing.T) {
	err := fmt.Errorf("plain error")
	st := errors.ToGRPCStatus(err)
	if st.Code() != codes.Internal {
		t.Errorf("ToGRPCStatus(plain) = %v, want Internal", st.Code())
	}
}

func TestToGRPCStatus_Message(t *testing.T) {
	err := errors.New(errors.CodeNotFound, "item missing")
	st := errors.ToGRPCStatus(err)
	if st.Message() != "item missing" {
		t.Errorf("ToGRPCStatus message = %q, want %q", st.Message(), "item missing")
	}
}

func TestAppError_Is(t *testing.T) {
	err := errors.New(errors.CodeNotFound, "not found")

	// errors.Is should match same code via Is method
	target := errors.New(errors.CodeNotFound, "different message")
	if !err.Is(target) {
		t.Error("Is() should match same ErrorCode regardless of message")
	}

	// Different code should not match
	other := errors.New(errors.CodeInternal, "server error")
	if err.Is(other) {
		t.Error("Is() should not match different ErrorCode")
	}

	// Non-AppError target should not match
	if err.Is(fmt.Errorf("plain")) {
		t.Error("Is() should not match non-AppError")
	}
}
