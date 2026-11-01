package platform_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/labstack/echo/v5"
)

func TestStarterSelectsEchoWithoutOpeningListener(t *testing.T) {
	for _, app := range []spine.App{platform.New(), spine.New(platform.Option())} {
		if err := app.Validate(boot.Options{HTTP: &boot.HTTPOptions{}}); err != nil {
			t.Fatalf("starter는 HTTP 구현을 선택해야 합니다: %v", err)
		}
	}
}

func TestStarterPreparesHTTPWithNativeEcho(t *testing.T) {
	app := platform.New()
	app.Transport(func(native any) {
		e, ok := native.(*echo.Echo)
		if !ok {
			t.Fatalf("native 훅은 Echo를 전달해야 합니다: %T", native)
		}
		e.GET("/native", func(ctx *echo.Context) error {
			return ctx.NoContent(http.StatusNoContent)
		})
	})
	prepared, err := spine.Prepare(context.Background(), app, boot.Options{
		Address: "prepare-must-not-listen",
		HTTP:    &boot.HTTPOptions{},
	})
	if err != nil {
		t.Fatal(err)
	}
	closeApplication := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := prepared.Close(ctx); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(closeApplication)

	recorder := httptest.NewRecorder()
	prepared.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/native", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("준비한 앱의 native 경로 응답: %d %s", recorder.Code, recorder.Body.String())
	}
	closeApplication()
	recorder = httptest.NewRecorder()
	prepared.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/native", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("Close 이후에는 새 요청을 거부해야 합니다: %d", recorder.Code)
	}
}
