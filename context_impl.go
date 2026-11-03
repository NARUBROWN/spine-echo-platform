package platform

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"strings"

	adapter "github.com/NARUBROWN/spine/adapter"
	"github.com/labstack/echo/v5"
)

type echoContext struct {
	echo   *echo.Context
	reqCtx context.Context
	writer adapter.ResponseWriter
}

var _ adapter.HTTPContext = (*echoContext)(nil)
var _ adapter.RawQueryContext = (*echoContext)(nil)

func newContext(c *echo.Context) adapter.HTTPContext {
	return &echoContext{echo: c, reqCtx: c.Request().Context(), writer: newEchoResponseWriter(c)}
}

func (c *echoContext) Context() context.Context       { return c.reqCtx }
func (c *echoContext) Method() string                 { return c.echo.Request().Method }
func (c *echoContext) Path() string                   { return c.echo.Request().URL.Path }
func (c *echoContext) Param(name string) string       { return c.echo.Param(name) }
func (c *echoContext) Query(name string) string       { return c.echo.QueryParam(name) }
func (c *echoContext) Queries() map[string][]string   { return c.echo.QueryParams() }
func (c *echoContext) RawQuery() (string, bool)       { return c.echo.Request().URL.RawQuery, true }
func (c *echoContext) Writer() adapter.ResponseWriter { return c.writer }

func (c *echoContext) Header(name string) string {
	if strings.EqualFold(name, "Host") {
		return c.echo.Request().Host
	}
	return c.echo.Request().Header.Get(name)
}

func (c *echoContext) Headers() map[string][]string {
	request := c.echo.Request()
	headers := request.Header.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	// Host는 Go가 헤더 맵에서 분리한 요청 필드를 사용합니다.
	// 조회 뷰만 보충하고 원래 요청 헤더는 변경하지 않습니다.
	for name := range headers {
		if strings.EqualFold(name, "Host") {
			delete(headers, name)
		}
	}
	if request.Host != "" {
		headers.Set("Host", request.Host)
	}
	return headers
}

func (c *echoContext) Params() map[string]string {
	values := c.echo.PathValues()
	params := make(map[string]string, len(values))
	for _, value := range values {
		params[value.Name] = value.Value
	}
	return params
}

func (c *echoContext) Bind(out any) error {
	if _, standard := c.echo.Echo().Binder.(*echo.DefaultBinder); !standard {
		return normalizeRequestError(c.echo.Bind(out))
	}
	request := c.echo.Request()
	media, _, _ := strings.Cut(request.Header.Get("Content-Type"), ";")
	if strings.TrimSpace(media) != echo.MIMEApplicationForm {
		return normalizeRequestError(echo.BindBody(c.echo, out))
	}
	// ParseForm의 URL 병합을 차단하고 이미 읽은 본문 값은 재사용합니다.
	bodyRequest := new(http.Request)
	*bodyRequest = *request
	url := *request.URL
	url.RawQuery = ""
	bodyRequest.URL = &url
	bodyRequest.Form = request.PostForm
	c.echo.SetRequest(bodyRequest)
	defer func() {
		if request.PostForm == nil {
			request.PostForm = bodyRequest.PostForm
		}
		c.echo.SetRequest(request)
	}()
	return normalizeRequestError(echo.BindBody(c.echo, out))
}

func (c *echoContext) MultipartForm() (*multipart.Form, error) {
	form, err := c.echo.MultipartForm()
	return form, normalizeRequestError(err)
}

func normalizeRequestError(err error) error {
	if err == nil {
		return nil
	}
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		return maxBytes
	}
	if status := echo.StatusCode(err); status != 0 {
		if status < 400 || status >= 500 {
			return err
		}
		message := http.StatusText(status)
		if message == "" {
			message = "Invalid request"
		}
		return &adapter.RequestError{Status: status, Message: message, Cause: err}
	}
	return &adapter.RequestError{Status: http.StatusBadRequest, Message: http.StatusText(http.StatusBadRequest), Cause: err}
}
