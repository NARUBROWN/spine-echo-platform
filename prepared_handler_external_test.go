package platform_test

import (
	"github.com/NARUBROWN/spine"
	"net/http"
	"testing"
)

func preparedHandler(t *testing.T, prepared spine.PreparedApplication) http.Handler {
	t.Helper()
	h, err := spine.NetHTTPHandler(prepared)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
