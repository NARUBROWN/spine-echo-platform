package platform_test

import (
	"context"
	"encoding/json"
	input "github.com/NARUBROWN/spine/pkg/input"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/query"
)

type queryInputs struct {
	Tags  []string `json:"tags" query:"tags"`
	IDs   []int    `json:"ids" query:"id"`
	Name  string   `json:"name" query:"name"`
	Count *int     `json:"count" query:"count"`
}
type queryController struct{ calls atomic.Int32 }

func (c *queryController) List(dto input.Query[queryInputs]) httpx.Response[queryInputs] {
	c.calls.Add(1)
	return httpx.Response[queryInputs]{Body: *dto}
}
func (c *queryController) Page(input query.Pagination) httpx.Response[query.Pagination] {
	c.calls.Add(1)
	return httpx.Response[query.Pagination]{Body: input}
}

func TestSpineHTTPQueryListsPreserveItemBoundariesAndSpaces(t *testing.T) {
	zero := 0
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "binding_only", true: "with_validator"}[configured], func(t *testing.T) {
			controller := &queryController{}
			var validations atomic.Int32
			options := []spine.Option{platform.Option()}
			if configured {
				options = append(options, spine.WithValidator(strictValidator(func(context.Context, any) error { validations.Add(1); return nil })))
			}
			app := spine.New(options...)
			app.Constructor(func() *queryController { return controller })
			app.Route("GET", "/query", (*queryController).List)
			app.Route("GET", "/page", (*queryController).Page)
			prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = prepared.Close(context.Background()) })
			server := httptest.NewServer(preparedHandler(t, prepared))
			t.Cleanup(server.Close)
			for _, test := range []struct {
				path   string
				want   queryInputs
				status int
			}{
				{"/query?tags=a&tags=b", queryInputs{Tags: []string{"a", "b"}}, 200},
				{"/query?tags=a,b", queryInputs{Tags: []string{"a", "b"}}, 200},
				{"/query?tags=a%2Cb", queryInputs{Tags: []string{"a,b"}}, 200},
				{"/query?tags=a,%20b", queryInputs{Tags: []string{"a", " b"}}, 200},
				{"/query?tags=a,b&%74ags=c%2Cd", queryInputs{Tags: []string{"a", "b", "c,d"}}, 200},
				{"/query?tags=a&tags=a,a", queryInputs{Tags: []string{"a", "a", "a"}}, 200},
				{"/query?tags=%20a%20,b+", queryInputs{Tags: []string{" a ", "b "}}, 200},
				{"/query?tags=,&tags=", queryInputs{Tags: []string{"", "", ""}}, 200},
				{"/query?tags=%252C", queryInputs{Tags: []string{"%2C"}}, 200},
				{"/query?id=1,2&id=3&count=0", queryInputs{IDs: []int{1, 2, 3}, Count: &zero}, 200},
				{"/query?name=a,b", queryInputs{Name: "a,b"}, 200},
				{"/query?unused=%zz&tags=a", queryInputs{Tags: []string{"a"}}, 200},
				{"/query?count=0&count=0", queryInputs{}, 400},
				{"/query?name=a&name=a", queryInputs{}, 400},
				{"/query?id=1,%202", queryInputs{}, 400},
				{"/query?id=1%2C2", queryInputs{}, 400},
				{"/query?tags=%zz", queryInputs{}, 400},
				{"/page?page=1&page=1", queryInputs{}, 400},
				{"/page?size=1&size=2", queryInputs{}, 400},
			} {
				beforeValidation, beforeHandler := validations.Load(), controller.calls.Load()
				response, err := server.Client().Get(server.URL + test.path)
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				wantCalls, wantValidations := int32(0), int32(0)
				if test.status == http.StatusOK {
					wantCalls = 1
					if configured {
						wantValidations = 1
					}
					var got queryInputs
					if err := json.Unmarshal(data, &got); err != nil || !reflect.DeepEqual(got, test.want) {
						t.Fatalf("URL=%s got=%+v want=%+v err=%v", test.path, got, test.want, err)
					}
				}
				if response.StatusCode != test.status || controller.calls.Load()-beforeHandler != wantCalls || validations.Load()-beforeValidation != wantValidations {
					t.Fatalf("URL=%s status=%d calls=%d validations=%d body=%s", test.path, response.StatusCode, controller.calls.Load()-beforeHandler, validations.Load()-beforeValidation, data)
				}
			}
		})
	}
}
