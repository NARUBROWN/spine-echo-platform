package platform

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"

	adapter "github.com/NARUBROWN/spine/adapter"
	"github.com/labstack/echo/v4"
)

type echoContext struct {
	echo   echo.Context
	reqCtx context.Context
	writer adapter.ResponseWriter
}

var _ adapter.HTTPContext = (*echoContext)(nil)

func newContext(c echo.Context) adapter.HTTPContext {
	return &echoContext{echo: c, reqCtx: c.Request().Context(), writer: newEchoResponseWriter(c)}
}

func (c *echoContext) Context() context.Context       { return c.reqCtx }
func (c *echoContext) Method() string                 { return c.echo.Request().Method }
func (c *echoContext) Path() string                   { return c.echo.Request().URL.Path }
func (c *echoContext) Param(name string) string       { return c.echo.Param(name) }
func (c *echoContext) Query(name string) string       { return c.echo.QueryParam(name) }
func (c *echoContext) Queries() map[string][]string   { return c.echo.QueryParams() }
func (c *echoContext) Header(name string) string      { return c.echo.Request().Header.Get(name) }
func (c *echoContext) Headers() map[string][]string   { return c.echo.Request().Header }
func (c *echoContext) Writer() adapter.ResponseWriter { return c.writer }

func (c *echoContext) Params() map[string]string {
	names, values := c.echo.ParamNames(), c.echo.ParamValues()
	params := make(map[string]string, len(names))
	for i, name := range names {
		if i < len(values) {
			params[name] = values[i]
		}
	}
	return params
}

func (c *echoContext) Bind(out any) error { return normalizeRequestError(c.echo.Bind(out)) }

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
	var echoError *echo.HTTPError
	if errors.As(err, &echoError) {
		if echoError.Code < 400 || echoError.Code >= 500 {
			return err
		}
		message := http.StatusText(echoError.Code)
		if message == "" {
			message = "Invalid request"
		}
		return &adapter.RequestError{Status: echoError.Code, Message: message, Cause: err}
	}
	return &adapter.RequestError{Status: http.StatusBadRequest, Message: http.StatusText(http.StatusBadRequest), Cause: err}
}
