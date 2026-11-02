package platform

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adapter "github.com/NARUBROWN/spine/adapter"
	"github.com/labstack/echo/v5"
)

func TestFactoryRejectsInvalidConfigBeforeHooks(t *testing.T) {
	for _, tt := range []struct {
		name               string
		change             func(*adapter.Config)
		configurationError bool
	}{
		{name: "nil_execute", change: func(c *adapter.Config) { c.Execute = nil }},
		{name: "empty_upgrade_path", change: func(c *adapter.Config) {
			c.Upgrades = []adapter.UpgradeRoute{{Handler: http.NotFoundHandler()}}
		}},
		{name: "nil_upgrade_handler", change: func(c *adapter.Config) {
			c.Upgrades = []adapter.UpgradeRoute{{Path: "/ws"}}
		}},
		{name: "typed_nil_upgrade_handler", change: func(c *adapter.Config) {
			c.Upgrades = []adapter.UpgradeRoute{{Path: "/ws", Handler: http.HandlerFunc(nil)}}
		}},
		{name: "negative_header_limit", configurationError: true, change: func(c *adapter.Config) { c.Options.MaxHeaderBytes = -1 }},
		{name: "negative_timeout", configurationError: true, change: func(c *adapter.Config) { c.Options.ReadTimeout = -time.Second }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			sdkOptions := optionsForHooks([]func(*echo.Echo){func(*echo.Echo) { calls++ }})
			config := adapter.Config{
				Execute: func(adapter.HTTPContext) error { return nil },
			}
			tt.change(&config)
			server, err := NewFactory(sdkOptions...).New(config)
			if server != nil || err == nil || calls != 0 {
				t.Fatalf("잘못된 입력은 훅 실행 전에 실패해야 합니다: server=%v err=%v calls=%d", server, err, calls)
			}
			if !errors.Is(err, adapter.ErrInvalidAdapter) {
				t.Fatalf("입력 오류 종류를 보존해야 합니다: %v", err)
			}
		})
	}
}

func TestFactoryCreatesIndependentHandlersWithoutRemounting(t *testing.T) {
	if err := adapter.ValidateFactory(NewFactory(), adapter.Requirements{WebSocketUpgrade: true}); err != nil {
		t.Fatal(err)
	}
	var natives []*echo.Echo
	sdkOptions := optionsForHooks([]func(*echo.Echo){func(value *echo.Echo) {
		natives = append(natives, value)
	}})
	config := adapter.Config{
		Execute: func(adapter.HTTPContext) error { return nil },
	}
	first := newConfiguredAdapter(t, config, sdkOptions...)
	second := newConfiguredAdapter(t, config, sdkOptions...)
	if first == second || first.Handler() == second.Handler() || first.Handler() != first.Handler() {
		t.Fatal("준비마다 독립 인스턴스를 만들고 Handler는 동일 진입점을 반환해야 합니다")
	}
	if len(natives) != 2 || natives[0] == natives[1] {
		t.Fatalf("훅은 준비마다 한 번만 호출해야 합니다: %d", len(natives))
	}
	if first.httpServer == second.httpServer || first.httpServer.Handler != first.echo || second.httpServer.Handler != second.echo {
		t.Fatal("각 어댑터는 해당 Echo handler를 사용하는 독립 HTTP 서버를 소유해야 합니다")
	}
}

func TestFactoryMountOrderAndHookSnapshot(t *testing.T) {
	var order []string
	var hooks []func(*echo.Echo)
	hooks = []func(*echo.Echo){
		func(value *echo.Echo) {
			order = append(order, "first")
			hooks[1] = func(*echo.Echo) { order = append(order, "mutated") }
			value.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(ctx *echo.Context) error {
					ctx.Response().Header().Set("X-Native", "yes")
					return next(ctx)
				}
			})
		},
		func(value *echo.Echo) {
			order = append(order, "second")
			value.GET("/ws/a", func(ctx *echo.Context) error { return ctx.String(200, "native") })
		},
	}
	var calls int
	var rawWriter http.ResponseWriter
	upgrades := []adapter.UpgradeRoute{
		{Path: "/ws/a", Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "upgrade-a") })},
		{Path: "/ws/b", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawWriter = w
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("업그레이드 경로에 fallback 본문 제한을 적용하면 안 됩니다: %v", err)
			}
			_, _ = io.WriteString(w, "upgrade-b:"+string(body))
		})},
	}
	sdkOptions := optionsForHooks(hooks)
	server := newConfiguredAdapter(t, adapter.Config{
		Options: adapter.ServerOptions{MaxBodyBytes: 1},
		Execute: func(ctx adapter.HTTPContext) error {
			calls++
			writer := ctx.Writer()
			return writer.WriteString(200, "fallback")
		},
		Upgrades: upgrades,
	}, sdkOptions...)
	upgrades[1].Handler = http.NotFoundHandler()
	if strings.Join(order, ",") != "first,second" {
		t.Fatalf("훅 목록을 snapshot으로 고정해야 합니다: %v", order)
	}
	for _, tt := range []struct{ path, want string }{
		{path: "/ws/a", want: "native"},
		{path: "/ws/b", want: "upgrade-b:body"},
		{path: "/api/ws/b", want: "fallback"},
	} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", tt.path, strings.NewReader("body")))
		if recorder.Body.String() != tt.want || recorder.Header().Get("X-Native") != "yes" {
			t.Fatalf("경로 우선순위 또는 middleware가 달라졌습니다: %s %s", tt.path, recorder.Body.String())
		}
		if tt.path == "/ws/b" && rawWriter != recorder {
			t.Fatal("업그레이드 handler에는 원래 HTTP writer를 전달해야 합니다")
		}
	}
	if calls != 1 || len(order) != 2 {
		t.Fatalf("fallback만 실행 콜백을 거치며 훅은 재등록하지 않아야 합니다: calls=%d hooks=%v", calls, order)
	}
}

func TestFactoryRequestScopeAndNoErrorRedelivery(t *testing.T) {
	type contextKey struct{}
	wantErr := errors.New("execution failed after response")
	var contexts []adapter.HTTPContext
	var errorCalls int
	sdkOptions := optionsForHooks([]func(*echo.Echo){func(value *echo.Echo) {
		e := value
		e.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		e.HTTPErrorHandler = func(*echo.Context, error) { errorCalls++ }
		e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(ctx *echo.Context) error { ctx.Set("native-value", true); return next(ctx) }
		})
	}})
	server := newConfiguredAdapter(t, adapter.Config{
		Execute: func(ctx adapter.HTTPContext) error {
			if ctx.Context().Value(contextKey{}) != "carried" {
				t.Error("원래 요청 context 값을 보존해야 합니다")
			}
			if ctx.Method() != "POST" || ctx.Path() != "/items" || ctx.Query("page") != "2" || ctx.Header("X-Request") != "value" {
				t.Error("원래 HTTP 입력을 보존해야 합니다")
			}
			contexts = append(contexts, ctx)
			writer := ctx.Writer()
			if err := writer.WriteString(202, "written-once"); err != nil {
				return err
			}
			return wantErr
		},
	}, sdkOptions...)
	for range 2 {
		request := httptest.NewRequest("POST", "/items?page=2", nil)
		request.Header.Set("X-Request", "value")
		request = request.WithContext(context.WithValue(request.Context(), contextKey{}, "carried"))
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != 202 || recorder.Body.String() != "written-once" {
			t.Fatalf("콜백 이후 응답을 다시 쓰면 안 됩니다: %d %s", recorder.Code, recorder.Body.String())
		}
	}
	if len(contexts) != 2 || contexts[0] == contexts[1] || errorCalls != 0 {
		t.Fatalf("요청마다 한 번 실행하고 Echo 오류 처리기로 재전달하지 않아야 합니다: contexts=%d errors=%d", len(contexts), errorCalls)
	}
}

func TestFactoryUpgradeReceivesRawWriterThroughEchoV5Wrapper(t *testing.T) {
	recorder := httptest.NewRecorder()
	var received http.ResponseWriter
	sdkOptions := optionsForHooks([]func(*echo.Echo){func(value *echo.Echo) {
		value.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(ctx *echo.Context) error {
				ctx.SetResponse(unwrapTestWriter{ctx.Response()})
				return next(ctx)
			}
		})
	}})
	server := newConfiguredAdapter(t, adapter.Config{
		Execute: func(adapter.HTTPContext) error {
			t.Fatal("업그레이드 경로가 Spine HTTP 실행 콜백을 호출하면 안 됩니다")
			return nil
		},
		Upgrades: []adapter.UpgradeRoute{{Path: "/ws", Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			received = w
			_, _ = io.WriteString(w, "raw-upgrade")
		})}},
	}, sdkOptions...)
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ws", nil))
	if received != recorder || recorder.Body.String() != "raw-upgrade" {
		t.Fatalf("업그레이드는 원래 writer를 받아야 합니다: %T %q", received, recorder.Body.String())
	}
}

func newConfiguredAdapter(t *testing.T, config adapter.Config, options ...EchoOption) *EchoAdapter {
	t.Helper()
	server, err := NewFactory(options...).New(config)
	if err != nil {
		t.Fatal(err)
	}
	return server.(*EchoAdapter)
}

func optionsForHooks(hooks []func(*echo.Echo)) []EchoOption {
	options := make([]EchoOption, 0, len(hooks))
	for _, hook := range hooks {
		options = append(options, WithEcho(hook))
	}
	return options
}

func TestTypedEchoOptionsAreLazyAndReusable(t *testing.T) {
	var natives []*echo.Echo
	options := []EchoOption{WithEcho(func(e *echo.Echo) { natives = append(natives, e) })}
	factory := NewFactory(options...)
	options[0] = WithEcho(func(*echo.Echo) { t.Fatal("변경된 옵션이 기존 factory에 반영되면 안 됩니다") })
	if err := adapter.ValidateFactory(factory, adapter.Requirements{}); err != nil || len(natives) != 0 {
		t.Fatalf("생성과 기능 검사는 Echo 설정을 실행하면 안 됩니다: %v", err)
	}
	for range 2 {
		if _, err := factory.New(adapter.Config{Execute: func(adapter.HTTPContext) error { return nil }}); err != nil {
			t.Fatal(err)
		}
	}
	if len(natives) != 2 || natives[0] == natives[1] {
		t.Fatal("준비마다 독립 Echo 인스턴스에 같은 설정을 한 번 적용해야 합니다")
	}
}

func TestFactoryRejectsNilEchoConfigurationBeforeHooks(t *testing.T) {
	calls := 0
	factory := NewFactory(WithEcho(func(*echo.Echo) { calls++ }), WithEcho(nil))
	server, err := factory.New(adapter.Config{Execute: func(adapter.HTTPContext) error { return nil }})
	if server != nil || !errors.Is(err, adapter.ErrInvalidAdapter) || calls != 0 {
		t.Fatalf("nil Echo 설정은 실행 전에 거부해야 합니다: server=%v err=%v calls=%d", server, err, calls)
	}
}
