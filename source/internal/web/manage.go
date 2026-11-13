package web

import (
	"expdeck/internal/core"
	"net/http"
	"net/url"
	"os"
	"time"
)

func (s *Server) handleOutcome(w http.ResponseWriter, r *http.Request) {
	exp, ok := s.findExperiment(w, r.FormValue("exp"))
	if !ok {
		return
	}
	id := r.FormValue("run")
	if err := s.kernel.SetRunOutcome(exp, id, r.FormValue("outcome")); err != nil {
		s.redirectError(w, r, exp, err)
		return
	}
	http.Redirect(w, r, newExpView(exp).URL+"?run="+url.QueryEscape(id), http.StatusSeeOther)
}
func (s *Server) handleRepresentative(w http.ResponseWriter, r *http.Request) {
	exp, ok := s.findExperiment(w, r.FormValue("exp"))
	if !ok {
		return
	}
	id := r.FormValue("run")
	if id == "auto" {
		id = ""
	}
	if err := s.kernel.SetRepresentative(exp, id); err != nil {
		s.redirectError(w, r, exp, err)
		return
	}
	target := newExpView(exp).URL
	if id != "" {
		target += "?run=" + url.QueryEscape(id)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单无效", http.StatusBadRequest)
		return
	}
	refs := r.PostForm["exp"]
	sel := make([]core.ExportSelection, 0, len(refs))
	for _, ref := range refs {
		exp, ok := s.findExperiment(w, ref)
		if !ok {
			return
		}
		choice := core.ExportSelection{Experiment: exp.ID}
		pick := r.FormValue("run_" + exp.ID)
		if pick == "all" {
			runs, err := s.kernel.ListRuns(exp)
			if err != nil {
				s.internalError(w, err)
				return
			}
			for _, run := range runs {
				if run.Status != core.RunRunning {
					choice.Runs = append(choice.Runs, run.ID)
				}
			}
			if len(choice.Runs) == 0 {
				http.Error(w, "没有可导出的运行记录", http.StatusBadRequest)
				return
			}
		}
		if pick != "" && pick != "all" {
			choice.Runs = []string{pick}
		}
		sel = append(sel, choice)
	}
	f, err := os.CreateTemp("", "expdeck-export-*.zip")
	if err != nil {
		s.internalError(w, err)
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()
	err = s.kernel.Export(f, sel, core.ExportOptions{Logs: r.FormValue("logs") == "1", Results: r.FormValue("results") == "1", FilterResults: true, Weights: r.FormValue("weights") == "1"})
	if err != nil {
		http.Error(w, "导出失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	if _, err = f.Seek(0, 0); err != nil {
		s.internalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="expdeck-export.zip"`)
	http.ServeContent(w, r, "expdeck-export.zip", fileTime(f), f)
}
func fileTime(f *os.File) (t time.Time) {
	if info, err := f.Stat(); err == nil {
		return info.ModTime()
	}
	return
}
