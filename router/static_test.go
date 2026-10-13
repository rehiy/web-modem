package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestSPAStaticServer(t *testing.T) {
	server := spaStaticServer{fs: http.FS(fstest.MapFS{
		"index.html":    {Data: []byte("<html>React app</html>")},
		"assets/app.js": {Data: []byte("console.log('app')")},
	})}
	for _, test := range []struct {
		path string
		body string
	}{
		{"/", "<html>React app</html>"},
		{"/euicc", "<html>React app</html>"},
		{"/assets/app.js", "console.log('app')"},
	} {
		t.Run(test.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != http.StatusOK || recorder.Body.String() != test.body {
				t.Fatalf("静态资源或 SPA 路由错误: %d %q", recorder.Code, recorder.Body.String())
			}
		})
	}
}
