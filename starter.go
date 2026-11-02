package platform

import "github.com/NARUBROWN/spine"

// Option은 Echo 통합을 명시적으로 선택하는 Spine 생성 옵션입니다.
// CLI composition에서는 spine.New(Option()) 형태로 같은 구성을 사용할 수 있습니다.
func Option(options ...EchoOption) spine.Option { return spine.WithHTTPAdapter(NewFactory(options...)) }

// New는 현재 모듈이 지원하는 Echo 어댑터를 주입한 Spine 앱을 반환합니다.
func New(options ...EchoOption) spine.App { return spine.New(Option(options...)) }
