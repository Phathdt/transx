package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
)

func httptestRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(""))
	return req
}
