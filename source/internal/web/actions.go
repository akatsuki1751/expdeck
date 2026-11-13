package web

import (
	"net/http"
	"net/url"
	"strings"
)

// expActions 是 /e/<ref>/<action> 这类写动作。
var expActions = map[string]bool{
	"attach":   true,
	"detach":   true,
	"conclude": true,
	"reopen":   true,
}

// splitAction 把 "a/b/detach" 拆成 (action="detach", ref="a/b")。
func splitAction(rest string) (action, ref string, ok bool) {
	idx := strings.LastIndex(rest, "/")
	if idx <= 0 || idx == len(rest)-1 {
		return "", "", false
	}
	action = rest[idx+1:]
	if !expActions[action] {
		return "", "", false
	}
	return action, rest[:idx], true
}

// handleExpAction 执行实验级动作，成功与失败都跳回详情页。
func (s *Server) handleExpAction(w http.ResponseWriter, r *http.Request, ref, action string) {
	exp, ok := s.findExperiment(w, ref)
	if !ok {
		return
	}
	var err error
	switch action {
	case "attach":
		_, err = s.kernel.Attach(exp)
	case "detach":
		_, err = s.kernel.Detach(exp)
	case "conclude":
		err = s.kernel.Conclude(exp, r.PostFormValue("decision"), r.PostFormValue("why"))
	case "reopen":
		err = s.kernel.Reopen(exp)
	default:
		s.notFound(w)
		return
	}
	if err != nil {
		s.redirectError(w, r, exp, err)
		return
	}
	http.Redirect(w, r, newExpView(exp).URL, http.StatusSeeOther)
}

// handleNew 建实验：成功直接进详情页。
func (s *Server) handleNew(w http.ResponseWriter, r *http.Request) {
	exp, err := s.kernel.Create(r.PostFormValue("path"), r.PostFormValue("why"))
	if err != nil {
		s.redirectIndexError(w, r, err)
		return
	}
	http.Redirect(w, r, newExpView(exp).URL, http.StatusSeeOther)
}

// handleAdopt 收编一条已经存在的 exp/ 分支。
func (s *Server) handleAdopt(w http.ResponseWriter, r *http.Request) {
	exp, err := s.kernel.Adopt(r.PostFormValue("branch"), r.PostFormValue("why"))
	if err != nil {
		s.redirectIndexError(w, r, err)
		return
	}
	http.Redirect(w, r, newExpView(exp).URL, http.StatusSeeOther)
}

// redirectIndexError 把失败原因带回列表页。
func (s *Server) redirectIndexError(w http.ResponseWriter, r *http.Request, err error) {
	http.Redirect(w, r, "/?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
}
