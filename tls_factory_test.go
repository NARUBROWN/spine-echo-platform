package platform

import (
	"crypto/tls"
	"errors"
	"testing"

	adapter "github.com/NARUBROWN/spine/contract/http"
	"github.com/labstack/echo/v5"
)

func TestTLSConfigFailsBeforeEchoHooks(t *testing.T) {
	calls := 0
	factory := NewFactory(WithEcho(func(*echo.Echo) { calls++ }))
	_, err := factory.New(adapter.Config{Execute: func(adapter.HTTPContext) error { return nil }, Serve: adapter.ServeOptions{TLS: &tls.Config{}}})
	if !errors.Is(err, adapter.ErrInvalidAdapter) || calls != 0 {
		t.Fatalf("TLS 검증 전 native 초기화: %v calls=%d", err, calls)
	}
}
