package platform_test

import (
	"context"
	input "github.com/NARUBROWN/spine/pkg/input"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type strictBody struct {
	Count int    `json:"count"`
	Flag  bool   `json:"flag"`
	Name  string `json:"name"`
	Maybe *int   `json:"maybe"`
}
type requiredBody struct {
	Count *int `json:"count" validate:"required"`
}
type requestHeaders struct {
	Host string `header:"Host" validate:"required"`
}
type strictController struct{ calls atomic.Int32 }

func (c *strictController) Body(body input.Body[strictBody]) httpx.Response[strictBody] {
	c.calls.Add(1)
	return httpx.Response[strictBody]{Body: *body}
}
func (c *strictController) Required(body input.Body[requiredBody]) httpx.Response[requiredBody] {
	c.calls.Add(1)
	return httpx.Response[requiredBody]{Body: *body}
}
func (c *strictController) Host(headers input.Header[requestHeaders]) httpx.Response[string] {
	c.calls.Add(1)
	return httpx.Response[string]{Body: headers.Host}
}

type strictValidator func(context.Context, any) error

func (f strictValidator) Validate(ctx context.Context, dto any) error { return f(ctx, dto) }

func TestSpineHTTPInputHostAndNull(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "binding_only", true: "with_validator"}[configured], func(t *testing.T) {
			controller := &strictController{}
			var validations atomic.Int32
			options := []spine.Option{platform.Option()}
			if configured {
				options = append(options, spine.WithValidator(strictValidator(func(_ context.Context, dto any) error {
					validations.Add(1)
					if value, ok := dto.(*requiredBody); ok && value.Count == nil {
						return validation.NewError(validation.FieldError{Field: "/count", Rule: "required"})
					}
					return nil
				})))
			}
			app := spine.New(options...)
			app.Constructor(func() *strictController { return controller })
			app.Route("POST", "/body", (*strictController).Body)
			app.Route("POST", "/required", (*strictController).Required)
			app.Route("GET", "/host", (*strictController).Host)
			prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = prepared.Close(context.Background()) })
			server := httptest.NewServer(preparedHandler(t, prepared))
			t.Cleanup(server.Close)
			for _, test := range []struct {
				path, body string
				status     int
				binds      bool
			}{
				{"/body", `{"count":null}`, 400, false},
				{"/body", `{"flag":null}`, 400, false},
				{"/body", `{"name":null}`, 400, false},
				{"/body", `null`, 400, false},
				{"/body", `{"count":0,"flag":false,"name":"","maybe":null}`, 200, true},
				{"/body", `{}`, 200, true},
				{"/required", `{"count":0}`, 200, true},
				{"/required", `{"count":null}`, map[bool]int{false: 200, true: 400}[configured], true},
			} {
				beforeValidation, beforeHandler := validations.Load(), controller.calls.Load()
				request, _ := http.NewRequest(http.MethodPost, server.URL+test.path, strings.NewReader(test.body))
				request.Header.Set("Content-Type", "application/json")
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				wantValidations := int32(0)
				if configured && test.binds {
					wantValidations = 1
				}
				wantHandled := int32(0)
				if test.status == 200 {
					wantHandled = 1
				}
				if response.StatusCode != test.status || validations.Load()-beforeValidation != wantValidations || controller.calls.Load()-beforeHandler != wantHandled {
					t.Fatalf("JSON=%s status=%d validations=%d handled=%d body=%s", test.body, response.StatusCode, validations.Load()-beforeValidation, controller.calls.Load()-beforeHandler, data)
				}
			}
			request, _ := http.NewRequest(http.MethodGet, server.URL+"/host", nil)
			request.Host = "api.example.com:8443"
			request.Header.Set("X-Forwarded-Host", "proxy.example")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			host, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != 200 || string(host) != request.Host {
				t.Fatalf("Host DTO가 실제 Request.Host를 받아야 합니다: status=%d host=%q err=%v", response.StatusCode, host, err)
			}
		})
	}
}
