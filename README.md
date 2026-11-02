# spine-echo-platform

Spine v0.6.0용 Echo v5.4.4 HTTP 플랫폼과 편의 생성자를 제공합니다. Go 패키지 이름은 `platform`이며 Go 1.25.5 이상을 사용합니다.

플랫폼 모듈은 Spine과 Echo를 의존성으로 가져오고, Spine의 공개 `adapter` 계약을 구현하는 EchoAdapter를 제공합니다. 계약은 Spine 모듈에 포함되며 별도 계약 모듈이나 저장소를 요구하지 않습니다.

## Spine CLI로 시작하기

Spine `v0.6.0`부터 새 프로젝트의 공식 생성 경로는 Spine CLI의 `new`입니다. [CLI `v0.4.0`](https://github.com/NARUBROWN/spine-cli/releases/tag/v0.4.0)은 Spine `v0.6.0`, Echo 플랫폼 `v0.1.0`과 Echo `v5.4.4`를 함께 구성합니다.

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@v0.4.0
spine version
spine new todo-api --module github.com/acme/todo-api
cd todo-api
spine dev
```

다른 터미널에서 `curl http://localhost:8080/health`로 `200 OK`와 `{"status":"ok"}` 응답을 확인합니다. 생성된 `go.mod`에는 Spine `v0.6.0`과 플랫폼 `v0.1.0`이 있어야 합니다.

`new`는 기본으로 플랫폼 연결, 의존성 설치, 정적 계약 검사와 빌드를 완료한 뒤 프로젝트를 생성합니다. 서버는 `spine dev`로 시작합니다. `--skip-install`로 파일만 생성했다면 `go mod tidy`를 별도로 실행하세요.

## 기존 앱에 직접 플랫폼 연결

기존 Go 모듈에 수동으로 연결할 때 플랫폼 의존성을 추가합니다.

```sh
go get github.com/NARUBROWN/spine-echo-platform@v0.1.0
```

```go
import (
    "log"

    "github.com/NARUBROWN/spine-echo-platform"
    "github.com/NARUBROWN/spine/pkg/boot"
)

func main() {
    app := platform.New()
    if err := app.Run(boot.Options{Address: ":8080", HTTP: &boot.HTTPOptions{}}); err != nil {
        log.Fatal(err)
    }
}
```

`platform.New()`는 `spine.New(platform.Option())`와 같습니다. 직접 조립할 때는 `spine.WithHTTPAdapter(factory)`를 사용할 수 있습니다. 커널의 spine.New()는 HTTP 플랫폼을 기본 선택하지 않습니다.

NewFactory는 `github.com/NARUBROWN/spine/adapter.Factory`를 제공합니다. 구성 시 fallback, WS GET 경로, native 훅 순으로 마운트합니다. DTO 바인딩과 multipart 파싱, 실제 응답 출력과 HTTP 서버 종료를 처리합니다. Spine의 응답 staging과 WS 메시지 엔진은 커널에 남습니다.

`WithEcho`는 `github.com/labstack/echo/v5`의 `*echo.Echo`를 받는 typed 플랫폼 옵션입니다. native 경로는 Spine의 HTTP 인터셉터, 본문 제한과 staging을 거치지 않습니다. native store와 Spine 요청 store는 별개이므로 공유 요청 값은 표준 context를 사용합니다.

## Native Echo v5 확장

v0.1.0은 Spine의 HTTP adapter v2 계약을 사용합니다. 생성자·컨트롤러 등록 API는 유지합니다. native Echo 확장 코드는 v5 import와 포인터 Context를 사용합니다.

```go
import "github.com/labstack/echo/v5"

app := platform.New(platform.WithEcho(func(e *echo.Echo) {
    e.GET("/native", func(ctx *echo.Context) error {
        return ctx.String(200, "native")
    })
}))
```

- handler와 middleware는 `*echo.Context`를 받습니다.
- `HTTPErrorHandler`는 `func(ctx *echo.Context, err error)` 순서입니다.
- logger는 표준 `log/slog`를 사용합니다.
- 경로 인자는 `PathValues()`로 조회합니다. `Response()`는 `http.ResponseWriter`를 반환하며 Echo의 상태가 필요하면 `echo.UnwrapResponse`를 사용합니다. native middleware의 응답 wrapper도 `Unwrap() http.ResponseWriter`를 제공해야 합니다.

`NewFactory(options...)`, `Option(options...)`와 `New(options...)`는 같은 Echo 옵션을 받습니다. 옵션 등록·기능 검사는 함수를 실행하지 않으며, 각 준비에서 fallback·WS 경로를 등록한 뒤 순서대로 한 번 실행합니다. nil 함수는 준비 단계에서 거부합니다.

응답 `WriteJSON`은 core에서 직렬화한 `json.RawMessage`만 받습니다. Echo의 JSON 출력 경로를 유지하므로 개행·pretty 출력과 사용자 JSONSerializer도 적용됩니다. 사용자 serializer에는 원래 DTO 대신 RawMessage가 전달되므로 이 타입을 처리해야 합니다. v1의 `Config.NativeHooks`와 `App.Transport`는 사용하지 않습니다.

플랫폼은 v5의 HTTP 상태 코드 인터페이스를 통해 입력 오류의 상태와 원인을 보존합니다. WS 업그레이드에는 원래 HTTP writer를 전달합니다. 서버 실행과 종료는 플랫폼이 소유한 `http.Server`가 처리합니다.

## 리스너 없이 앱 준비

`platform.New()`가 반환하는 `spine.App`은 `app.Prepare(ctx, options)`를 직접 제공합니다. HTTP 옵션을 지정하면 준비 결과의 `Handler()`를 외부 HTTP 서버나 `httptest`에 연결할 수 있습니다. 준비 과정에서는 리스너를 열거나 consumer와 custom transport를 시작하지 않습니다. 설정에 따라 broker readiness와 custom Init은 실행될 수 있습니다.

준비 결과는 호출자가 `Close(ctx)`로 정리합니다. Close를 시작하면 새 요청은 503을 받고, 진행 중인 요청이 끝난 뒤 프레임워크 자원을 정리합니다. 대기 시간이 초과돼도 정리는 계속되므로 다시 Close를 호출해 완료를 기다릴 수 있습니다. 외부 서버의 listener는 호출자가 종료합니다.

API와 자원 소유권은 [Spine v0.6.0 전환 안내](https://github.com/NARUBROWN/spine/blob/v0.6.0/docs/migration/http-adapter-modules.md#리스너-없이-앱-준비하기)를 참고하세요.

## 개발 저장소 구성

```text
VscodeProjects/
  spine/
  spine-echo-platform/
```

go.mod는 공개된 Spine `v0.6.0`을 사용하며 로컬 replace를 포함하지 않습니다. 커널과 함께 개발하려면 위 배치에서 go work init . ../spine으로 로컬 workspace를 만들거나, 기존 go.work의 replace에 기능 worktree 경로를 지정합니다. 로컬 go.work는 커밋하지 않습니다.

v0.1.0은 Spine `v0.6.0`을 사용합니다. go.work와 로컬 replace 없이 고정한 공개 버전으로 설치하고 빌드합니다.

[관련 이슈](https://github.com/NARUBROWN/spine/issues/9)와 [최소 E2E 예제](https://github.com/NARUBROWN/spine/tree/v0.6.0/cmd/demo)를 참고하세요.

```sh
GOWORK=off go mod verify
GOWORK=off go test -race ./... -count=1
```

공개 CI는 go.work와 로컬 replace 없이 고정한 공개 Spine 버전으로 검사합니다.
