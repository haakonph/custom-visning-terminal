package usage

import (
	"net/http"
	"strconv"
)

// HTTPHandler counts completed incoming requests, including rejected requests.
// Only explicitly known paths and standard methods become metric labels.
func (r *Recorder) HTTPHandler(next http.Handler, knownPaths ...string) http.Handler {
	if r == nil {
		return next
	}
	paths := make(map[string]bool, len(knownPaths))
	for _, path := range knownPaths {
		paths[path] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := "unknown"
		if paths[req.URL.Path] {
			path = req.URL.Path
		}
		method := req.Method
		switch method {
		case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		default:
			method = "OTHER"
		}
		response := &httpResponse{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(response, req)
		r.Record("http.incoming", method, path, strconv.Itoa(response.status))
	})
}

type httpResponse struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *httpResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *httpResponse) WriteHeader(status int) {
	if !w.wroteHeader && (status >= 200 || status == http.StatusSwitchingProtocols) {
		w.status = status
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *httpResponse) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(body)
}
