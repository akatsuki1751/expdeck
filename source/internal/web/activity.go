package web

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var auditMu sync.Mutex

// actionLog deliberately excludes query strings, form values, tokens, and notes.
func (s *Server) actionLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		capture := &actionResponse{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(capture, r)
		outcome := "ok"
		if capture.status >= 400 || strings.Contains(capture.Header().Get("Location"), "error=") {
			outcome = "error"
		}
		event := map[string]any{"time": time.Now().Format(time.RFC3339), "method": r.Method, "path": r.URL.Path, "status": capture.status, "result": outcome}
		// Keep a useful error reason, redacting submitted form values.
		if outcome == "error" {
			if location, parseErr := url.Parse(capture.Header().Get("Location")); parseErr == nil {
				reason := location.Query().Get("error")
				_ = r.ParseForm()
				for _, values := range r.PostForm {
					for _, value := range values {
						if len(value) >= 3 {
							reason = strings.ReplaceAll(reason, value, "[redacted]")
						}
					}
				}
				if len(reason) > 500 {
					reason = reason[:500]
				}
				if reason != "" {
					event["reason"] = reason
				}
			}
		}
		line, err := json.Marshal(event)
		if err != nil {
			return
		}
		auditMu.Lock()
		defer auditMu.Unlock()
		path := filepath.Join(s.kernel.Store.Logs(), "web.log")
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			log.Printf("web audit: %v", err)
			return
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			log.Printf("web audit: %v", err)
			return
		}
		defer f.Close()
		if _, err = f.Write(append(line, '\n')); err != nil {
			log.Printf("web audit: %v", err)
		}
	})
}

type actionResponse struct {
	http.ResponseWriter
	status int
}

func (w *actionResponse) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	path := filepath.Join(s.kernel.Store.Logs(), "web.log")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		s.internalError(w, err)
		return
	}
	if len(data) > 256*1024 {
		data = data[len(data)-256*1024:]
		if n := strings.IndexByte(string(data), '\n'); n >= 0 {
			data = data[n+1:]
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}
