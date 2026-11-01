// Package platform은 Spine 공개 HTTP 계약의 Echo 구현과 플랫폼 구성을 제공합니다.
package platform

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	adapter "github.com/NARUBROWN/spine/adapter"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
)

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 30 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultIdleTimeout       = 120 * time.Second
	defaultMaxHeaderBytes    = 1 << 20
	defaultMaxBodyBytes      = 32 << 20
)

type normalizedHTTPOptions struct {
	DisableRecover    bool
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	MaxBodyBytes      int64
}

type lifecycleState uint8

const (
	stateReady lifecycleState = iota
	stateServing
	stateStopping
	stateStopped
)

// EchoAdapter는 HTTP 처리와 실제 서버 실행을 분리합니다. NewFactory로 생성합니다.
// Handler를 외부 서버에 연결했다면 해당 서버의 종료는 외부 소유자가 관리합니다.
type EchoAdapter struct {
	echo         *echo.Echo
	execute      adapter.ExecuteFunc
	httpServer   *http.Server
	maxBodyBytes int64

	lifecycleMu sync.Mutex
	state       lifecycleState
}

var _ adapter.HTTPAdapter = (*EchoAdapter)(nil)

func newAdapter(execute adapter.ExecuteFunc, opts adapter.ServerOptions) *EchoAdapter {
	normalized := normalizeHTTPOptions(opts)
	e := newEcho(normalized)

	httpServer := &http.Server{
		Handler:           e,
		ReadHeaderTimeout: normalized.ReadHeaderTimeout,
		ReadTimeout:       normalized.ReadTimeout,
		WriteTimeout:      normalized.WriteTimeout,
		IdleTimeout:       normalized.IdleTimeout,
		MaxHeaderBytes:    normalized.MaxHeaderBytes,
	}

	return &EchoAdapter{
		echo:         e,
		execute:      execute,
		httpServer:   httpServer,
		maxBodyBytes: normalized.MaxBodyBytes,
	}
}

func normalizeHTTPOptions(opts adapter.ServerOptions) normalizedHTTPOptions {
	normalized := normalizedHTTPOptions{
		DisableRecover:    opts.DisableRecover,
		ReadHeaderTimeout: opts.ReadHeaderTimeout,
		ReadTimeout:       opts.ReadTimeout,
		WriteTimeout:      opts.WriteTimeout,
		IdleTimeout:       opts.IdleTimeout,
		MaxHeaderBytes:    opts.MaxHeaderBytes,
		MaxBodyBytes:      opts.MaxBodyBytes,
	}

	if normalized.ReadHeaderTimeout == 0 {
		normalized.ReadHeaderTimeout = defaultReadHeaderTimeout
	}
	if normalized.ReadTimeout == 0 {
		normalized.ReadTimeout = defaultReadTimeout
	}
	if normalized.WriteTimeout == 0 {
		normalized.WriteTimeout = defaultWriteTimeout
	}
	if normalized.IdleTimeout == 0 {
		normalized.IdleTimeout = defaultIdleTimeout
	}
	if normalized.MaxHeaderBytes == 0 {
		normalized.MaxHeaderBytes = defaultMaxHeaderBytes
	}
	if normalized.MaxBodyBytes == 0 {
		normalized.MaxBodyBytes = defaultMaxBodyBytes
	}

	return normalized
}

func newEcho(opts normalizedHTTPOptions) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Logger.SetLevel(log.ERROR)
	if !opts.DisableRecover {
		e.Use(simpleRecover())
	}
	return e
}

// simpleRecover는 외부 의존 없이 패닉을 500 응답으로 변환하는 최소한의 미들웨어입니다.
func simpleRecover() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			defer func() {
				if r := recover(); r != nil {
					c.Logger().Errorf("panic recovered: %v", r)
					_ = c.JSON(http.StatusInternalServerError, map[string]any{
						"message": "Internal server error",
					})
				}
			}()
			return next(c)
		}
	}
}

// Handler는 초기화를 반복하지 않고 준비된 동일 HTTP 진입점을 반환합니다.
func (s *EchoAdapter) Handler() http.Handler {
	if s == nil || s.echo == nil {
		return nil
	}
	return s.echo
}

// Serve는 리스너를 소유하고 인스턴스당 한 번 서버를 실행합니다.
// 실제 서버 실행 중에는 상태 잠금을 유지하지 않습니다.
func (s *EchoAdapter) Serve(listener net.Listener) error {
	if isNil(listener) {
		return fmt.Errorf("%w: listener is nil", adapter.ErrInvalidState)
	}
	if s == nil || s.httpServer == nil {
		return errors.Join(fmt.Errorf("%w: EchoAdapter is not initialized", adapter.ErrInvalidAdapter), listener.Close())
	}

	s.lifecycleMu.Lock()
	if s.state != stateReady {
		s.lifecycleMu.Unlock()
		return errors.Join(fmt.Errorf("%w: Serve requires a new adapter", adapter.ErrInvalidState), listener.Close())
	}
	s.state = stateServing
	s.lifecycleMu.Unlock()
	defer func() {
		s.lifecycleMu.Lock()
		if s.state != stateStopped {
			s.state = stateStopping
		}
		s.lifecycleMu.Unlock()
	}()

	// http.Server.Serve는 정상 종료와 오류를 포함한 모든 반환 경로에서 리스너를 닫습니다.
	return s.httpServer.Serve(listener)
}

func (s *EchoAdapter) handle(c echo.Context) error {
	if s.maxBodyBytes > 0 {
		req := c.Request()
		req.Body = http.MaxBytesReader(c.Response(), req.Body, s.maxBodyBytes)
		c.SetRequest(req)
	}

	ctx := newContext(c)

	if err := s.execute(ctx); err != nil {
		c.Logger().Errorf("HTTP execution error: %v", err)
		// 파이프라인 내부에서 이미 응답이 작성되었으므로 Echo 기본 에러 핸들러로 중복 전달하지 않는다.
		return nil
	}
	return nil
}

// Shutdown은 HTTP 종료를 기다립니다. 시간 초과 뒤 새 context로 다시 기다릴 수 있습니다.
func (s *EchoAdapter) Shutdown(ctx context.Context) error {
	if isNil(ctx) {
		return fmt.Errorf("%w: shutdown context is nil", adapter.ErrInvalidState)
	}
	if s == nil || s.httpServer == nil {
		return fmt.Errorf("%w: EchoAdapter is not initialized", adapter.ErrInvalidAdapter)
	}
	s.lifecycleMu.Lock()
	if s.state == stateStopped {
		s.lifecycleMu.Unlock()
		return nil
	}
	s.state = stateStopping
	s.lifecycleMu.Unlock()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return err
	}
	s.lifecycleMu.Lock()
	s.state = stateStopped
	s.lifecycleMu.Unlock()
	return nil
}
