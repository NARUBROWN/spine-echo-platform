package platform_test

import (
	"testing"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	"github.com/NARUBROWN/spine/pkg/boot"
)

func TestStarterSelectsEchoWithoutOpeningListener(t *testing.T) {
	for _, app := range []spine.App{platform.New(), spine.New(platform.Option())} {
		if err := app.Validate(boot.Options{HTTP: &boot.HTTPOptions{}}); err != nil {
			t.Fatalf("starter는 HTTP 구현을 선택해야 합니다: %v", err)
		}
	}
}
