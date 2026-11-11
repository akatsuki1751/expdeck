package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
)

var page = template.Must(template.New("page").Parse(`<!doctype html><html lang="zh"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Expedition</title><style>body{margin:0;background:#f5f5f3;color:#202622;font:15px system-ui}header{background:#fff;border-bottom:1px solid #ddd;padding:18px 5%}main{max-width:1100px;margin:30px auto;padding:0 20px}h1{font-size:23px;margin:0}h2{font-size:19px}article,section{background:white;border:1px solid #ddd;border-radius:8px;padding:20px;margin:16px 0}input,textarea{box-sizing:border-box;border:1px solid #bbb;border-radius:4px;padding:9px;font:inherit;width:100%}textarea{min-height:120px;resize:vertical}button{background:#285c49;color:white;border:0;border-radius:4px;padding:9px 14px;cursor:pointer}label{display:block;margin:12px 0 5px}small{color:#59645d}form.inline{display:inline} .grid{display:grid;grid-template-columns:1fr 2fr;gap:10px}.msg{color:#9b3629;white-space:pre-wrap}pre{white-space:pre-wrap;overflow-wrap:anywhere;font:inherit}</style></head><body><header><h1>Expedition</h1><small>实验与笔记</small></header><main>{{if .Message}}<p class="msg">{{.Message}}</p>{{end}}<section><h2>新建实验</h2><form method="post" action="/create"><input type="hidden" name="token" value="{{.Token}}"><div class="grid"><input name="branch" placeholder="路径，如 model/attention" required><input name="hypothesis" placeholder="假设（可选）"></div><p><button>创建实验</button></p></form></section><h2>实验</h2>{{range .Experiments}}<article><h2>{{.Branch}}</h2><p><strong>假设：</strong>{{if .Hypothesis}}{{.Hypothesis}}{{else}}未填写{{end}}</p><p><strong>工作树：</strong>{{if .Worktree}}{{.Worktree}}{{else}}未挂载{{end}}</p><form method="post" action="/note"><input type="hidden" name="token" value="{{$.Token}}"><input type="hidden" name="branch" value="{{.Branch}}"><label>笔记</label><textarea name="notes">{{.Notes}}</textarea><p><button>保存笔记</button></p></form><form class="inline" method="post" action="/{{if .Worktree}}detach{{else}}attach{{end}}"><input type="hidden" name="token" value="{{$.Token}}"><input type="hidden" name="branch" value="{{.Branch}}"><button>{{if .Worktree}}卸载{{else}}挂载{{end}}</button></form></article>{{else}}<p>还没有实验。</p>{{end}}</main></body></html>`))

func serve(addr string) error {
	host, _, e := net.SplitHostPort(addr)
	if e != nil {
		return e
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("网页仅支持监听本机地址；远程访问请使用 SSH 端口转发")
	}
	raw := make([]byte, 32)
	if _, e = rand.Read(raw); e != nil {
		return e
	}
	token := hex.EncodeToString(raw)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return
		}
		xs, e := list()
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		page.Execute(w, struct {
			Token, Message string
			Experiments    []experiment
		}{token, r.URL.Query().Get("message"), xs})
	})
	for _, action := range []string{"create", "note", "attach", "detach"} {
		action := action
		mux.HandleFunc("/"+action, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				http.Error(w, "method not allowed", 405)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			if e := r.ParseForm(); e != nil {
				http.Error(w, "invalid form", 400)
				return
			}
			if r.Form.Get("token") != token {
				http.Error(w, "forbidden", 403)
				return
			}
			b, e := branch(r.Form.Get("branch"))
			if e == nil {
				switch action {
				case "create":
					e = create(b, r.Form.Get("hypothesis"))
				case "note":
					if !exists(b) {
						e = fmt.Errorf("实验不存在")
					} else {
						rec := readRecord(b)
						rec.Notes = r.Form.Get("notes")
						e = save(rec)
					}
				case "attach":
					e = attach(b, "")
				case "detach":
					e = detach(b)
				}
			}
			if e != nil {
				http.Redirect(w, r, "/?message="+urlQuery(e.Error()), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
	}
	ln, e := net.Listen("tcp", addr)
	if e != nil {
		return e
	}
	fmt.Println("网页地址: http://" + ln.Addr().String())
	return http.Serve(ln, mux)
}
func urlQuery(s string) string { return url.QueryEscape(s) }
