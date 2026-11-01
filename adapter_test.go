package platform

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adapter "github.com/NARUBROWN/spine/adapter"
	"github.com/labstack/echo/v4"
)

func TestContextBind_NormalizesMalformedJSONToBadRequest(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	ctx := newContext(e.NewContext(req, httptest.NewRecorder())).(*echoContext)

	err := ctx.Bind(&struct {
		Name string `json:"name"`
	}{})
	var httpErr *adapter.RequestError
	if !errors.As(err, &httpErr) || httpErr.Status != http.StatusBadRequest {
		t.Fatalf("잘못된 JSON은 400 Spine 오류여야 합니다: %T %v", err, err)
	}
}

func TestNormalizeRequestError_PreservesMaxBytesError(t *testing.T) {
	maxBytesErr := &http.MaxBytesError{Limit: 64}
	err := normalizeRequestError(echo.NewHTTPError(http.StatusBadRequest).SetInternal(maxBytesErr))
	var got *http.MaxBytesError
	if !errors.As(err, &got) || got.Limit != 64 {
		t.Fatalf("요청 크기 초과 오류를 보존해야 합니다: %T %v", err, err)
	}
}

func TestNormalizeRequestError_MapsRawRequestErrorToBadRequest(t *testing.T) {
	err := normalizeRequestError(errors.New("malformed multipart body"))
	var httpErr *adapter.RequestError
	if !errors.As(err, &httpErr) || httpErr.Status != http.StatusBadRequest {
		t.Fatalf("요청 파싱 오류는 400 Spine 오류여야 합니다: %T %v", err, err)
	}
}

func TestNewFactory_AppliesSecureDefaults(t *testing.T) {
	server := newTestAdapter(t, adapter.ServerOptions{})

	if server.httpServer.ReadHeaderTimeout != defaultReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout 기본값이 적용되지 않았습니다: %s", server.httpServer.ReadHeaderTimeout)
	}
	if server.httpServer.ReadTimeout != defaultReadTimeout {
		t.Fatalf("ReadTimeout 기본값이 적용되지 않았습니다: %s", server.httpServer.ReadTimeout)
	}
	if server.httpServer.WriteTimeout != defaultWriteTimeout {
		t.Fatalf("WriteTimeout 기본값이 적용되지 않았습니다: %s", server.httpServer.WriteTimeout)
	}
	if server.httpServer.IdleTimeout != defaultIdleTimeout {
		t.Fatalf("IdleTimeout 기본값이 적용되지 않았습니다: %s", server.httpServer.IdleTimeout)
	}
	if server.httpServer.MaxHeaderBytes != defaultMaxHeaderBytes {
		t.Fatalf("MaxHeaderBytes 기본값이 적용되지 않았습니다: %d", server.httpServer.MaxHeaderBytes)
	}
}

func TestNewFactory_AppliesCustomOptions(t *testing.T) {
	server := newTestAdapter(t, adapter.ServerOptions{
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      4 * time.Second,
		IdleTimeout:       5 * time.Second,
		MaxHeaderBytes:    4096,
		MaxBodyBytes:      128,
	})

	if server.httpServer.ReadHeaderTimeout != 2*time.Second {
		t.Fatalf("ReadHeaderTimeout가 반영되지 않았습니다: %s", server.httpServer.ReadHeaderTimeout)
	}
	if server.httpServer.ReadTimeout != 3*time.Second {
		t.Fatalf("ReadTimeout가 반영되지 않았습니다: %s", server.httpServer.ReadTimeout)
	}
	if server.httpServer.WriteTimeout != 4*time.Second {
		t.Fatalf("WriteTimeout가 반영되지 않았습니다: %s", server.httpServer.WriteTimeout)
	}
	if server.httpServer.IdleTimeout != 5*time.Second {
		t.Fatalf("IdleTimeout가 반영되지 않았습니다: %s", server.httpServer.IdleTimeout)
	}
	if server.httpServer.MaxHeaderBytes != 4096 {
		t.Fatalf("MaxHeaderBytes가 반영되지 않았습니다: %d", server.httpServer.MaxHeaderBytes)
	}
}

func newTestAdapter(t *testing.T, options adapter.ServerOptions) *EchoAdapter {
	t.Helper()
	server, err := NewFactory().New(adapter.Config{
		Options: options,
		Execute: func(adapter.HTTPContext) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return server.(*EchoAdapter)
}
