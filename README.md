# spine-echo-platform

> HTTP 계약 `spine.http-adapter/v3`와 공통 입력 파서를 사용합니다. 이 소스의 Preview 조합은 Echo `v0.2.0-preview.3`와 코어 `v0.7.0-preview.4`입니다. HTTP v2 조합에는 소급 적용되지 않습니다. CLI `v0.4.3-preview.4`부터 Echo와 net/http 생성 및 업데이트를 지원합니다. 코어의 [HTTP v3 전환 안내](https://github.com/NARUBROWN/spine/blob/preview/docs/migration/http-v3.md)를 참고하세요.


> 이 작업 소스의 대상 버전은 `v0.2.0`이며 Preview, RC, 정식 채널로 발행합니다. [릴리스 운영](RELEASING.md)을 확인하세요. 기존 정식 `v0.1.0`은 변경하지 않습니다.

Spine HTTP v3용 Echo v5.4.4 플랫폼과 편의 생성자를 제공합니다. Go 패키지 이름은 `platform`이며 Go 1.25.5 이상을 사용합니다.

플랫폼 모듈은 Spine과 Echo를 의존성으로 가져오고, Spine의 공개 `contract/http` 계약을 구현하는 EchoAdapter를 제공합니다. 계약은 Spine 모듈에 포함되며 별도 계약 모듈이나 저장소를 요구하지 않습니다.

## Spine CLI로 시작하기

이 절과 아래 `v0.1.0` 직접 설치 예시는 기존 공개 조합을 설명합니다. 이번 HTTP v3 후보를 생성하는 CLI 지원은 후속 작업입니다.

Spine `v0.6.0`부터 새 프로젝트의 공식 생성 경로는 Spine CLI의 `new`입니다. [최신 공개 CLI](https://github.com/NARUBROWN/spine-cli#권장-환경과-설치)를 설치하면, `new`가 CLI에 포함된 검증 레시피를 따라 Spine, Echo 플랫폼과 Echo의 호환 버전 조합을 구성합니다.

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@latest
spine version
spine new todo-api --module github.com/acme/todo-api
cd todo-api
spine dev
```

다른 터미널에서 `curl http://localhost:8080/health`로 `200 OK`와 `{"status":"ok"}` 응답을 확인합니다. 선택된 의존성 버전은 생성된 `go.mod`에서 확인합니다.

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

NewFactory는 `github.com/NARUBROWN/spine/contract/http.Factory`를 제공합니다. 구성 시 fallback, WS GET 경로, native 훅 순으로 마운트합니다. 플랫폼은 원시 요청 입력, 실제 응답 출력과 HTTP 서버 종료를 처리합니다. DTO 바인딩, multipart 파싱, 응답 staging과 WS 메시지 엔진은 커널이 담당합니다.

`WithEcho`는 `github.com/labstack/echo/v5`의 `*echo.Echo`를 받는 typed 플랫폼 옵션입니다. native 경로는 Spine의 HTTP 인터셉터, 본문 제한과 staging을 거치지 않습니다. native store와 Spine 요청 store는 별개이므로 공유 요청 값은 표준 context를 사용합니다.

## TLS 실행

현재 v3 개발 소스에서는 `boot.HTTPOptions.TLS`로 관리 서버의 TLS를 설정합니다. `crypto/tls`로 인증서를 먼저 읽습니다.

```go
certificate, err := tls.LoadX509KeyPair("server.crt", "server.key")
if err != nil {
    return err
}
return app.RunContext(ctx, boot.Options{
    Address: ":8443",
    HTTP: &boot.HTTPOptions{
        TLS: &tls.Config{Certificates: []tls.Certificate{certificate}},
    },
})
```

TLS가 nil이면 평문 HTTP를 사용합니다. 주소를 생략하면 TLS는 `:https`, 평문은 `:http`입니다. Prepare는 TLS 지원과 정적 인증서를 native 훅 실행 전에 검사합니다. 인증서 선택 콜백은 실제 연결 때 호출합니다. 설정과 인증서 바이트는 복사하며 개인 키 객체와 콜백의 안전한 동시 사용은 호출자가 보장합니다.

준비한 어댑터의 `Serve(listener)`는 같은 설정을 사용합니다. 실행 시 `httpcontract.WithTLS(config)`를 추가하면 기본값을 교체하며 마지막 옵션이 우선합니다. `httpcontract`는 `github.com/NARUBROWN/spine/contract/http`의 import 별칭입니다. `WithTLS(nil)`과 nil 옵션은 오류이며 공개 `ServeTLS`는 추가하지 않습니다. 외부 서버에 handler만 연결한 경우 TLS는 외부 서버 소유입니다. HTTP 버전 선택과 ALPN 정책은 이번 변경에 포함하지 않습니다.

## Native Echo v5 확장

v0.1.0은 Spine의 HTTP adapter v2 계약을 사용합니다. 생성자, 컨트롤러 등록 API는 유지합니다. native Echo 확장 코드는 v5 import와 포인터 Context를 사용합니다.

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

`NewFactory(options...)`, `Option(options...)`와 `New(options...)`는 같은 Echo 옵션을 받습니다. 옵션 등록, 기능 검사는 함수를 실행하지 않으며, 각 준비에서 fallback, WS 경로를 등록한 뒤 순서대로 한 번 실행합니다. nil 함수는 준비 단계에서 거부합니다.

응답 `WriteJSON`은 core에서 직렬화한 `json.RawMessage`만 받습니다. Echo의 JSON 출력 경로를 유지하므로 개행, pretty 출력과 사용자 JSONSerializer도 적용됩니다. 사용자 serializer에는 원래 DTO 대신 RawMessage가 전달되므로 이 타입을 처리해야 합니다. v1의 `Config.NativeHooks`와 `App.Transport`는 사용하지 않습니다.

플랫폼은 v5의 HTTP 상태 코드 인터페이스를 통해 입력 오류의 상태와 원인을 보존합니다. WS 업그레이드에는 원래 HTTP writer를 전달합니다. 서버 실행과 종료는 플랫폼이 소유한 `http.Server`가 처리합니다.

## 개발 브랜치의 입력 바인딩

Host 조회는 Go의 `Request.Host`를 별도로 읽습니다. `Header("Host")`와 `Headers()["Host"]`는 같은 요청 값을 제공하며 대소문자가 다른 Host 이름도 같은 값으로 처리합니다. 포트는 보존하고 `X-Forwarded-Host`로 대체하지 않습니다. 조회 맵을 변경해도 원래 요청 헤더는 변경되지 않습니다.

v3의 필수 `RawQuery`는 `Request.URL.RawQuery`를 디코딩하지 않고 코어로 전달합니다. 코어의 쿼리 슬라이스는 반복 키와 쉼표 구분을 함께 지원합니다. `tags=a,b`는 두 원소, `tags=a%2Cb`는 쉼표를 포함한 한 원소로 바인딩하며 `tags=a,%20b`의 두 번째 원소는 앞 공백을 유지합니다. 일반 Query와 Queries 조회도 코어가 URL 디코딩해 제공합니다.

기본 JSON 디코더는 Spine의 `pkg/jsonx.Unmarshal`을 사용합니다. `int`, `bool`, `string`과 일반 구조체, 배열에 보낸 명시적 null은 400 입력 오류로 거부합니다. 중첩 필드와 컬렉션 원소에도 같은 규칙을 적용합니다. 필드 누락, 포인터와 map, slice의 null은 보존하고 필수 여부는 validator가 판단합니다. 사용자 정의 `json.Unmarshaler`는 자신의 null 처리를 유지합니다.

Spine DTO 입력은 Echo Binder와 JSONSerializer를 호출하지 않습니다. `WithEcho`의 Binder 교체는 native Echo 경로에서 사용하고, 응답 JSONSerializer는 기존 Echo 출력 경로에 적용합니다. 요청 크기 제한과 HTTP 오류 변환은 유지합니다.

이 변경은 공개된 `v0.1.0`에 포함되지 않습니다. `pkg/jsonx`를 포함한 Spine 개발 브랜치와 로컬 workspace에서 함께 검증해야 하며, 공개 배포 전에 해당 Spine 버전으로 의존성을 갱신해야 합니다.

## 리스너 없이 앱 준비

`platform.New()`가 반환하는 `spine.App`은 `app.Prepare(ctx, options)`를 직접 제공합니다. HTTP 옵션을 지정하면 `spine.NetHTTPHandler(prepared)`로 얻은 handler를 외부 HTTP 서버나 `httptest`에 연결할 수 있습니다. 준비 과정에서는 리스너를 열거나 consumer와 custom transport를 시작하지 않습니다. 설정에 따라 broker readiness와 custom Init은 실행될 수 있습니다.

준비 결과는 호출자가 `Close(ctx)`로 정리합니다. Close를 시작하면 새 요청은 503을 받고, 진행 중인 요청이 끝난 뒤 프레임워크 자원을 정리합니다. 대기 시간이 초과돼도 정리는 계속되므로 다시 Close를 호출해 완료를 기다릴 수 있습니다. 외부 서버의 listener는 호출자가 종료합니다.

API와 자원 소유권은 [Spine v0.6.0 전환 안내](https://github.com/NARUBROWN/spine/blob/v0.6.0/docs/migration/http-adapter-modules.md#리스너-없이-앱-준비하기)를 참고하세요.

## 개발 저장소 구성

```text
VscodeProjects/
  spine/
  spine-echo-platform/
```

현재 작업 소스의 go.mod는 Spine `v0.7.0-preview.4`을 사용하며 로컬 replace를 포함하지 않습니다. 소스 CI는 고정한 코어 커밋을 사용합니다. 공개 태그의 검증 상태는 release manifest를 확인하세요. 발행 전후 검증은 [릴리스 운영](RELEASING.md)을 따릅니다. 커널과 함께 개발하려면 위 배치에서 go work init . ../spine으로 로컬 workspace를 만들거나, 기존 go.work의 replace에 기능 worktree 경로를 지정합니다. 로컬 go.work는 커밋하지 않습니다.

v0.1.0은 Spine `v0.6.0`을 사용합니다. go.work와 로컬 replace 없이 고정한 공개 버전으로 설치하고 빌드합니다.

[관련 이슈](https://github.com/NARUBROWN/spine/issues/9)와 [최소 E2E 예제](https://github.com/NARUBROWN/spine/tree/v0.6.0/cmd/demo)를 참고하세요.

```sh
GOWORK=off go mod verify
GOWORK=off go test -race ./... -count=1
```

공개 CI는 go.work와 로컬 replace 없이 고정한 공개 Spine 버전으로 검사합니다.
