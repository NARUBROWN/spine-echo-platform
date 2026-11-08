// Package platform은 Spine 공개 HTTP 계약의 Echo 구현과 플랫폼 구성을 제공합니다.
package platform

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	adapter "github.com/NARUBROWN/spine/contract/http"
	"github.com/labstack/echo/v5"
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
	serveOptions adapter.ServeOptions

	lifecycleMu  sync.Mutex
	state        lifecycleState
	handler      http.Handler
	active       int
	drained      chan struct{}
	shutdownDone chan struct{}
	shutdownOnce sync.Once
	shutdownErr  error
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

	server := &EchoAdapter{
		echo:         e,
		execute:      execute,
		httpServer:   httpServer,
		maxBodyBytes: normalized.MaxBodyBytes,
		drained:      make(chan struct{}), shutdownDone: make(chan struct{}),
	}
	server.handler = server
	server.httpServer.Handler = server.handler
	return server
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
	e := echo.NewWithConfig(echo.Config{
		Logger:         slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})),
		JSONSerializer: jsonSerializer{},
	})
	if !opts.DisableRecover {
		e.Use(simpleRecover())
	}
	return e
}

// simpleRecover는 외부 의존 없이 패닉을 500 응답으로 변환하는 최소한의 미들웨어입니다.
func simpleRecover() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			defer func() {
				if r := recover(); r != nil {
					c.Logger().ErrorContext(c.Request().Context(), "panic recovered", "panic", r)
					_ = c.JSON(http.StatusInternalServerError, struct {
						Message string `json:"message"`
					}{Message: "Internal server error"})
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
	return s.handler
}

// Serve는 리스너를 소유하고 인스턴스당 한 번 서버를 실행합니다.
// 실제 서버 실행 중에는 상태 잠금을 유지하지 않습니다.
func (s *EchoAdapter) Serve(listener net.Listener, options ...adapter.ServeOption) (serveErr error) {
	if isNil(listener) {
		return fmt.Errorf("%w: listener is nil", adapter.ErrInvalidState)
	}
	owned := &ownedListener{Listener: listener}
	defer func() {
		if closeErr := owned.Close(); closeErr != nil {
			serveErr = errors.Join(serveErr, closeErr)
		}
	}()
	if s == nil || s.httpServer == nil {
		return fmt.Errorf("%w: EchoAdapter is not initialized", adapter.ErrInvalidAdapter)
	}
	resolved, err := s.serveOptions.Resolve((factory{}).Capabilities(), options...)
	if err != nil {
		return err
	}

	s.lifecycleMu.Lock()
	if s.state != stateReady {
		s.lifecycleMu.Unlock()
		return fmt.Errorf("%w: Serve requires a new adapter", adapter.ErrInvalidState)
	}
	s.state = stateServing
	s.lifecycleMu.Unlock()

	var native net.Listener = owned
	if resolved.TLS != nil {
		native = tls.NewListener(owned, resolved.TLS)
	}
	err = s.httpServer.Serve(native)
	if errors.Is(err, http.ErrServerClosed) {
		return adapter.ErrServerClosed
	}
	return err
}

type ownedListener struct {
	net.Listener
	once sync.Once
	err  error
}

func (l *ownedListener) Close() error {
	l.once.Do(func() { l.err = l.Listener.Close() })
	return l.err
}

func (s *EchoAdapter) handle(c *echo.Context) error {
	if s.maxBodyBytes > 0 {
		req := c.Request()
		req.Body = http.MaxBytesReader(c.Response(), req.Body, s.maxBodyBytes)
		c.SetRequest(req)
	}

	ctx := newContext(c)

	if err := s.execute(ctx); err != nil {
		if !isCommittedNotFound(c) {
			c.Logger().ErrorContext(c.Request().Context(), "HTTP execution error", "error", err)
		}
		// 파이프라인 내부에서 이미 응답이 작성되었으므로 Echo 기본 에러 핸들러로 중복 전달하지 않는다.
		return nil
	}
	return nil
}

// 이미 렌더링된 404는 상위 native 미들웨어가 정적 파일이나 CSR 응답으로 대체할 수 있는 정상 제어 흐름입니다.
func isCommittedNotFound(c *echo.Context) bool {
	response, err := echo.UnwrapResponse(c.Response())
	return err == nil && response.Committed && response.Status == http.StatusNotFound
}

// ServeHTTP는 native middleware와 업그레이드까지 포함한 요청 전체를 추적합니다.
func (s *EchoAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.lifecycleMu.Lock()
	if s.state >= stateStopping {
		s.lifecycleMu.Unlock()
		http.Error(w, "application is closing", http.StatusServiceUnavailable)
		return
	}
	s.active++
	s.lifecycleMu.Unlock()
	ctx, cancel := context.WithCancel(r.Context())
	defer func() {
		cancel()
		s.lifecycleMu.Lock()
		defer s.lifecycleMu.Unlock()
		s.active--
		if s.state >= stateStopping && s.active == 0 {
			close(s.drained)
		}
	}()
	s.echo.ServeHTTP(w, r.WithContext(ctx))
}

// StopAccepting은 진행 요청을 보존하고 새 native 진입을 동기 차단합니다.
func (s *EchoAdapter) StopAccepting() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.state >= stateStopping {
		return
	}
	s.state = stateStopping
	if s.active == 0 {
		close(s.drained)
	}
}

// Shutdown의 context는 대기만 제한합니다. 정리는 백그라운드에서 계속합니다.
func (s *EchoAdapter) Shutdown(ctx context.Context) error {
	if isNil(ctx) {
		return fmt.Errorf("%w: shutdown context is nil", adapter.ErrInvalidState)
	}
	if s == nil || s.httpServer == nil {
		return fmt.Errorf("%w: EchoAdapter is not initialized", adapter.ErrInvalidAdapter)
	}
	s.StopAccepting()
	s.shutdownOnce.Do(func() {
		go func() {
			s.shutdownErr = s.httpServer.Shutdown(context.Background())
			<-s.drained
			s.lifecycleMu.Lock()
			s.state = stateStopped
			s.lifecycleMu.Unlock()
			close(s.shutdownDone)
		}()
	})
	select {
	case <-s.shutdownDone:
		return s.shutdownErr
	default:
	}
	select {
	case <-s.shutdownDone:
		return s.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
