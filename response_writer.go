package platform

import (
	adapter "github.com/NARUBROWN/spine/adapter"
	"github.com/labstack/echo/v4"
)

type echoResponseWriter struct {
	ctx echo.Context
}

var _ adapter.ResponseWriter = (*echoResponseWriter)(nil)

func newEchoResponseWriter(ctx echo.Context) *echoResponseWriter {
	return &echoResponseWriter{ctx: ctx}
}

func (w *echoResponseWriter) WriteJSON(status int, value any) error {
	return w.ctx.JSON(status, value)
}

func (w *echoResponseWriter) WriteString(status int, value string) error {
	return w.ctx.String(status, value)
}

func (w *echoResponseWriter) SetHeader(key, value string) {
	w.ctx.Response().Header().Set(key, value)
}

func (w *echoResponseWriter) AddHeader(key, value string) {
	w.ctx.Response().Header().Add(key, value)
}

func (w *echoResponseWriter) IsCommitted() bool {
	return w.ctx.Response().Committed
}

func (w *echoResponseWriter) WriteStatus(status int) error {
	w.ctx.Response().WriteHeader(status)
	return nil
}

func (w *echoResponseWriter) WriteBytes(status int, value []byte) error {
	return w.ctx.Blob(status, w.ctx.Response().Header().Get("Content-Type"), value)
}
