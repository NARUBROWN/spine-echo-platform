package platform

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestHostViewsUseOnlyRequestHost(t *testing.T) {
	for _, host := range []string{"api.example.com:8443", "[::1]:8080", ""} {
		t.Run(host, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Host = host
			request.Header = http.Header{
				"Host": {"ignored.example"}, "host": {"also-ignored.example"},
				"X-Forwarded-Host": {"proxy.example"}, "X-ID": {"original"},
			}
			original := request.Header.Clone()
			scope := newContext(newEcho(normalizedHTTPOptions{}).NewContext(request, httptest.NewRecorder()))
			for _, name := range []string{"Host", "HOST", "host"} {
				if got := scope.Header(name); got != host {
					t.Fatalf("Host는 Request.Host만 읽어야 합니다: %q != %q", got, host)
				}
			}
			view := scope.Headers()
			if values := view["Host"]; host != "" && (len(values) != 1 || values[0] != host) || host == "" && len(values) != 0 {
				t.Fatalf("Host 조회 뷰가 일치해야 합니다: %+v", view)
			}
			if _, exists := view["host"]; exists {
				t.Fatal("헤더 맵의 별도 Host 값이 중복으로 남으면 안 됩니다")
			}
			view["X-ID"][0] = "changed"
			view["Host"] = []string{"changed"}
			if !reflect.DeepEqual(original, request.Header) || scope.Header("Host") != host {
				t.Fatal("조회 뷰를 바꿔도 원래 요청을 변경하면 안 됩니다")
			}
		})
	}
}

func TestRawInputSnapshotHasNoParserSideEffects(t *testing.T) {
	request := httptest.NewRequest("POST", "/a%252Fb/../c?tags=a%2Cb,c", strings.NewReader("not parsed"))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
	native := newEcho(normalizedHTTPOptions{}).NewContext(request, httptest.NewRecorder())
	scope := newContext(native)
	if scope.RawPath() != "/a%252Fb/../c" || scope.RawQuery() != "tags=a%2Cb,c" {
		t.Fatal("원시 입력을 정리하거나 디코딩하면 안 됩니다")
	}
	if request.MultipartForm != nil || request.PostForm != nil {
		t.Fatal("getter에서 본문을 파싱하면 안 됩니다")
	}
	native.SetRequest(httptest.NewRequest("GET", "/reused", nil))
	data, err := io.ReadAll(scope.Body())
	if err != nil || string(data) != "not parsed" || scope.RawPath() != "/a%252Fb/../c" {
		t.Fatal("native 재사용과 본문 위치를 분리해야 합니다")
	}
}
