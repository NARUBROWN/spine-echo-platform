package platform_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"

	platform "github.com/NARUBROWN/spine-echo-platform"
	adapter "github.com/NARUBROWN/spine/adapter"
)

func ExampleNewFactory() {
	server, err := platform.NewFactory().New(adapter.Config{
		Execute: func(ctx adapter.HTTPContext) error {
			writer := ctx.Writer()
			body, err := json.Marshal(map[string]string{"method": ctx.Method()})
			if err != nil {
				return err
			}
			return writer.WriteJSON(200, body)
		},
	})
	if err != nil {
		panic(err)
	}

	// 공개 계약과 Echo 구현만으로 리스너 없이 요청을 처리할 수 있습니다.
	recorder := httptest.NewRecorder()
	handler, err := adapter.NetHTTPHandler(server)
	if err != nil {
		panic(err)
	}
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/", nil))
	fmt.Println(recorder.Code)
	fmt.Print(recorder.Body.String())

	// Output:
	// 200
	// {"method":"GET"}
}
