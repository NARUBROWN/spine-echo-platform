package platform

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adapter "github.com/NARUBROWN/spine/adapter"
	"github.com/labstack/echo/v5"
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
	err := normalizeRequestError(echo.NewHTTPError(http.StatusBadRequest, http.StatusText(http.StatusBadRequest)).Wrap(maxBytesErr))
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

func TestNormalizeRequestError_PreservesEchoV5StatusAndCause(t *testing.T) {
	cause := errors.New("invalid field")
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{
		{name: "status_sentinel", err: echo.ErrUnsupportedMediaType, status: http.StatusUnsupportedMediaType},
		{name: "wrapped_sentinel", err: fmt.Errorf("bind: %w", echo.ErrBadRequest), status: http.StatusBadRequest},
		{name: "wrapped_HTTP_error", err: echo.NewHTTPError(http.StatusUnprocessableEntity, "invalid field").Wrap(cause), status: http.StatusUnprocessableEntity},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := normalizeRequestError(tt.err)
			var input *adapter.RequestError
			if !errors.As(err, &input) || input.Status != tt.status || input.Message != http.StatusText(tt.status) {
				t.Fatalf("Echo v5 상태를 보존해야 합니다: %T %v", err, err)
			}
			if !errors.Is(err, tt.err) {
				t.Fatalf("원래 Echo 오류를 보존해야 합니다: %v", err)
			}
			if tt.name == "wrapped_HTTP_error" && !errors.Is(err, cause) {
				t.Fatalf("오류의 원인까지 보존해야 합니다: %v", err)
			}
		})
	}
	serverError := echo.NewHTTPError(http.StatusInternalServerError, "server failed")
	if got := normalizeRequestError(serverError); got != serverError {
		t.Fatalf("서버 오류를 입력 오류로 바꾸면 안 됩니다: %v", got)
	}
}

func TestContextParams_CopyEchoV5PathValues(t *testing.T) {
	native := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/items/43", nil), httptest.NewRecorder())
	native.SetPathValues(echo.PathValues{{Name: "id", Value: "43"}})
	scope := newContext(native)
	if scope.Param("id") != "43" || scope.Params()["id"] != "43" {
		t.Fatal("Echo v5 PathValues를 계약의 경로 인자로 전달해야 합니다")
	}
	params := scope.Params()
	params["id"] = "changed"
	if scope.Param("id") != "43" || scope.Params()["id"] != "43" {
		t.Fatal("반환한 경로 인자 맵을 바꿔도 native 값은 유지돼야 합니다")
	}
}

type unwrapTestWriter struct{ http.ResponseWriter }

func (w unwrapTestWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestResponseWriterTracksEchoV5WrappedResponseCommit(t *testing.T) {
	for _, nativeWrite := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		native := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), recorder)
		native.SetResponse(unwrapTestWriter{native.Response()})
		writer := newEchoResponseWriter(native)
		if writer.IsCommitted() {
			t.Fatal("출력 전에는 응답이 커밋되지 않아야 합니다")
		}
		if nativeWrite {
			native.Response().WriteHeader(http.StatusAccepted)
		} else if err := writer.WriteString(http.StatusAccepted, "written"); err != nil {
			t.Fatal(err)
		}
		if !writer.IsCommitted() || recorder.Code != http.StatusAccepted {
			t.Fatalf("wrapper를 거쳐도 실제 커밋 상태를 읽어야 합니다: native=%v code=%d", nativeWrite, recorder.Code)
		}
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
