# spine-echo-platform

Echo 기반 HTTP 플랫폼과 Spine 편의 생성자를 제공합니다. Go 패키지 이름은 `platform`입니다.

플랫폼 모듈은 Spine과 Echo를 의존성으로 가져오고, Spine의 공개 `adapter` 계약을 구현하는 EchoAdapter를 제공합니다. 계약은 Spine 모듈에 포함되며 별도 계약 모듈이나 저장소를 요구하지 않습니다.

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

app.Transport는 기존 *echo.Echo를 전달합니다. native 경로는 Spine의 HTTP 인터셉터, 본문 제한과 staging을 거치지 않습니다. native store와 Spine 요청 store는 별개이므로 공유 요청 값은 표준 context를 사용합니다.

## 리스너 없이 앱 준비

`platform.New()`로 만든 앱은 `spine.Prepare(ctx, app, options)`를 지원합니다. HTTP 옵션을 지정하면 준비 결과의 `Handler()`를 외부 HTTP 서버나 `httptest`에 연결할 수 있습니다. 준비 과정에서는 리스너를 열거나 consumer와 custom transport를 시작하지 않습니다. 설정에 따라 broker readiness와 custom Init은 실행될 수 있습니다.

준비 결과는 호출자가 `Close(ctx)`로 정리합니다. Close를 시작하면 새 요청은 503을 받고, 진행 중인 요청이 끝난 뒤 프레임워크 자원을 정리합니다. 대기 시간이 초과돼도 정리는 계속되므로 다시 Close를 호출해 완료를 기다릴 수 있습니다. 외부 서버의 listener는 호출자가 종료합니다.

API와 자원 소유권은 [Spine 전환 안내](https://github.com/NARUBROWN/spine/blob/490135eda06ff82acbb0c4f28eab9b7f1bf518a6/docs/migration/http-adapter-modules.md#리스너-없이-앱-준비하기)를 참고하세요.

## 개발 저장소 구성

```text
VscodeProjects/
  spine/
  spine-echo-platform/
```

go.mod는 공개된 Spine 커밋의 Go 의사버전을 사용하며 로컬 replace를 포함하지 않습니다. 커널과 함께 개발하려면 위 배치에서 go work init . ../spine으로 로컬 workspace를 만들거나, 기존 go.work의 replace에 기능 worktree 경로를 지정합니다. 로컬 go.work는 커밋하지 않습니다.

현재는 이슈 #9의 개발 스냅샷이며 정식 릴리스 태그는 아직 없습니다. Spine 의존성은 Prepare API를 포함하는 `v0.5.3-0.20261101161224-490135eda06f`으로 고정합니다. 의존성 설치와 테스트는 로컬 replace 없이도 실행할 수 있습니다.

개발 스냅샷은 다음 명령으로 가져옵니다. go get은 현재 main 커밋을 Go 의사버전으로 기록합니다.

```sh
go get github.com/NARUBROWN/spine-echo-platform@main
```

[관련 이슈](https://github.com/NARUBROWN/spine/issues/9)와 [최소 E2E 예제](https://github.com/NARUBROWN/spine/tree/feat/9-echo-adapter-contract/cmd/demo)를 참고하세요.

```sh
GOWORK=off go mod verify
GOWORK=off go test -race ./... -count=1
```

CI도 go.work와 로컬 replace 없이 공개 Spine 버전으로 모듈 정합성, vet와 race 테스트를 검사합니다.
