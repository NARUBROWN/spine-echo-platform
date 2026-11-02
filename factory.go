package platform

import (
	"fmt"
	"reflect"
	"slices"

	adapter "github.com/NARUBROWN/spine/adapter"
	"github.com/labstack/echo/v5"
)

type factory struct {
	hooks []func(*echo.Echo)
}

// EchoOption은 Echo 인스턴스를 구성하는 플랫폼 전용 옵션입니다.
type EchoOption func(*factory)

// WithEcho는 각 준비 과정에서 라우트 등록 후 한 번 호출할 설정 함수를 추가합니다.
// 옵션과 factory 생성은 함수를 실행하지 않으며 nil 함수는 준비 단계에서 거부합니다.
func WithEcho(configure func(*echo.Echo)) EchoOption {
	return func(f *factory) { f.hooks = append(f.hooks, configure) }
}

var _ adapter.Factory = factory{}

// NewFactory는 HTTP 실행, WS 업그레이드 마운트와 native 훅을 지원하는 factory를 반환합니다.
// factory 생성과 기능 조회는 리스너나 서버 실행을 시작하지 않습니다.
func NewFactory(options ...EchoOption) adapter.Factory {
	f := factory{}
	for _, option := range options {
		if option != nil {
			option(&f)
		}
	}
	f.hooks = slices.Clone(f.hooks)
	return f
}

func (factory) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{
		ContractVersion:  adapter.ContractVersion,
		WebSocketUpgrade: true,
	}
}

func (f factory) New(config adapter.Config) (adapter.HTTPAdapter, error) {
	upgrades := slices.Clone(config.Upgrades)
	hooks := slices.Clone(f.hooks)
	if config.Execute == nil {
		return nil, fmt.Errorf("%w: Execute is nil", adapter.ErrInvalidAdapter)
	}
	if err := config.Options.Validate(); err != nil {
		return nil, err
	}
	for i, route := range upgrades {
		if route.Path == "" || isNil(route.Handler) {
			return nil, fmt.Errorf("%w: Upgrades[%d] requires a path and handler", adapter.ErrInvalidAdapter, i)
		}
	}
	for i, hook := range hooks {
		if hook == nil {
			return nil, fmt.Errorf("%w: Echo configuration[%d] is nil", adapter.ErrInvalidAdapter, i)
		}
	}

	server := newAdapter(config.Execute, config.Options)
	server.echo.Any("/*", server.handle)
	for _, route := range upgrades {
		server.echo.GET(route.Path, func(c *echo.Context) error {
			response, err := echo.UnwrapResponse(c.Response())
			if err != nil {
				return err
			}
			route.Handler.ServeHTTP(response.ResponseWriter, c.Request())
			return nil
		})
	}
	for _, hook := range hooks {
		hook(server.echo)
	}
	return server, nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
