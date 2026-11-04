package platform

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	adapter "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/pkg/jsonx"
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

func TestJSONBindingRejectsScalarNullWithRequestError(t *testing.T) {
	for _, data := range []string{`{"count":null}`, `{"child":{"count":null}}`, `{"items":[null]}`, `null`} {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(data))
		request.Header.Set("Content-Type", "application/json")
		scope := newContext(newEcho(normalizedHTTPOptions{}).NewContext(request, httptest.NewRecorder()))
		var body struct {
			Count int `json:"count"`
			Child struct {
				Count int `json:"count"`
			} `json:"child"`
			Items []int `json:"items"`
		}
		err := scope.Bind(&body)
		var requestErr *adapter.RequestError
		var null *jsonx.NullError
		if !errors.As(err, &requestErr) || requestErr.Status != http.StatusBadRequest || !errors.As(err, &null) {
			t.Fatalf("일반 값의 null은 원인을 보존한 400이어야 합니다: %s %v", data, err)
		}
	}
}

func TestRawQueryPreservesEscapingBeforeDecodedViews(t *testing.T) {
	for _, raw := range []string{"tags=a%2Cb,%20c&tags=d", ""} {
		request := httptest.NewRequest(http.MethodGet, "/?"+raw, nil)
		scope := newContext(newEcho(normalizedHTTPOptions{}).NewContext(request, httptest.NewRecorder()))
		source, ok := scope.(adapter.RawQueryContext)
		if !ok {
			t.Fatal("원시 쿼리 조회 계약을 제공해야 합니다")
		}
		if got, available := source.RawQuery(); !available || got != raw {
			t.Fatalf("빈 쿼리와 인코딩을 보존해야 합니다: %q %v", got, available)
		}
		if raw != "" && !reflect.DeepEqual(scope.Queries()["tags"], []string{"a,b, c", "d"}) {
			t.Fatal("기존 Queries는 디코딩된 뷰를 유지해야 합니다")
		}
	}
}

func TestDefaultBindingReadsOnlyFormBodyAndRestoresNativeRequest(t *testing.T) {
	for _, parsed := range []bool{false, true} {
		request := httptest.NewRequest("POST", "/?id=from-query", strings.NewReader("other=body"))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if parsed {
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
		}
		ctx := newEcho(normalizedHTTPOptions{}).NewContext(request, httptest.NewRecorder())
		scope := newContext(ctx)
		var body struct {
			ID string `form:"id"`
		}
		if err := scope.Bind(&body); err != nil || body.ID != "" {
			t.Fatalf("쿼리를 폼 본문에 병합하면 안 됩니다: %+v %v", body, err)
		}
		if ctx.Request() != request || scope.Query("id") != "from-query" {
			t.Fatal("native 요청과 별도 쿼리 조회를 보존해야 합니다")
		}
		if err := request.ParseForm(); err != nil || request.Form.Get("other") != "body" {
			t.Fatalf("후속 native 본문 조회: %v", err)
		}
	}
	request := httptest.NewRequest("POST", "/?id=%ZZ", strings.NewReader("id=from-body"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	scope := newContext(newEcho(normalizedHTTPOptions{}).NewContext(request, httptest.NewRecorder()))
	var body struct {
		ID string `form:"id"`
	}
	if err := scope.Bind(&body); err != nil || body.ID != "from-body" {
		t.Fatalf("쿼리 인코딩 오류가 본문 바인딩에 영향을 주면 안 됩니다: %+v %v", body, err)
	}
}
