package platform

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	adapter "github.com/NARUBROWN/spine/contract/http"
	"github.com/labstack/echo/v5"
)

type echoContext struct {
	reqCtx              context.Context
	method, path, query string
	headers             http.Header
	body                io.Reader
	writer              adapter.ResponseWriter
}

var _ adapter.HTTPContext = (*echoContext)(nil)

func newContext(c *echo.Context) adapter.HTTPContext {
	r := c.Request()
	headers := r.Header.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	for key := range headers {
		if strings.EqualFold(key, "Host") {
			delete(headers, key)
		}
	}
	if r.Host != "" {
		headers.Set("Host", r.Host)
	}
	return &echoContext{reqCtx: r.Context(), method: r.Method, path: r.URL.EscapedPath(), query: r.URL.RawQuery, headers: headers, body: requestReader{r.Body}, writer: newEchoResponseWriter(c)}
}
func (c *echoContext) Context() context.Context       { return c.reqCtx }
func (c *echoContext) Method() string                 { return c.method }
func (c *echoContext) RawPath() string                { return c.path }
func (c *echoContext) RawQuery() string               { return c.query }
func (c *echoContext) Header(name string) string      { return c.headers.Get(name) }
func (c *echoContext) Headers() map[string][]string   { return c.headers.Clone() }
func (c *echoContext) Body() io.Reader                { return c.body }
func (c *echoContext) Writer() adapter.ResponseWriter { return c.writer }

type requestReader struct{ source io.Reader }

func (r requestReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		err = &adapter.RequestError{Status: http.StatusRequestEntityTooLarge, Cause: err}
	}
	return n, err
}
