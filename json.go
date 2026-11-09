package platform

import (
	"bytes"
	"sync"

	"github.com/NARUBROWN/spine/pkg/jsonx"
	"github.com/labstack/echo/v5"
)

// JSON 입력에는 Spine의 공통 null 규칙을 적용하고 응답 인코딩은 Echo에 맡깁니다.
type jsonSerializer struct{ echo.DefaultJSONSerializer }

var jsonBuffers = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func (jsonSerializer) Deserialize(c *echo.Context, target any) error {
	buffer := jsonBuffers.Get().(*bytes.Buffer)
	buffer.Reset()
	defer func() {
		// 큰 요청의 버퍼가 풀에 계속 남지 않도록 제한합니다.
		if buffer.Cap() <= 64<<10 {
			jsonBuffers.Put(buffer)
		}
	}()
	if _, err := buffer.ReadFrom(c.Request().Body); err != nil {
		if echo.StatusCode(err) != 0 {
			return err
		}
		return echo.ErrBadRequest.Wrap(err)
	}
	if err := jsonx.Unmarshal(buffer.Bytes(), target); err != nil {
		return echo.ErrBadRequest.Wrap(err)
	}
	return nil
}
