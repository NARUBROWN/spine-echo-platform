package platform

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adapter "github.com/NARUBROWN/spine/adapter"
	"github.com/labstack/echo/v5"
)

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

func TestHandleSuppressesOnlyCommittedNotFoundExecutionLog(t *testing.T) {
	for _, tt := range []struct {
		name    string
		status  int
		wantLog bool
	}{
		{name: "committed_not_found", status: http.StatusNotFound, wantLog: false},
		{name: "other_client_error", status: http.StatusBadRequest, wantLog: true},
		{name: "server_error", status: http.StatusInternalServerError, wantLog: true},
		{name: "unrendered_error", wantLog: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			factory := NewFactory(WithEcho(func(server *echo.Echo) {
				server.Logger = slog.New(slog.NewTextHandler(&output, nil))
			}))
			prepared, err := factory.New(adapter.Config{Execute: func(ctx adapter.HTTPContext) error {
				if tt.status != 0 {
					if err := ctx.Writer().WriteString(tt.status, http.StatusText(tt.status)); err != nil {
						return err
					}
				}
				return errors.New("execution failed")
			}})
			if err != nil {
				t.Fatal(err)
			}

			recorder := httptest.NewRecorder()
			prepared.(*EchoAdapter).Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/fallback", nil))
			logged := strings.Contains(output.String(), "HTTP execution error")
			if logged != tt.wantLog {
				t.Fatalf("실행 오류 로그 경계가 다릅니다: status=%d logged=%v output=%q", tt.status, logged, output.String())
			}
			if tt.status != 0 && recorder.Code != tt.status {
				t.Fatalf("응답 상태를 바꾸면 안 됩니다: got=%d want=%d", recorder.Code, tt.status)
			}
		})
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
