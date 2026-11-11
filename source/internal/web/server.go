// Package web 是 ExpDeck 的浏览与笔记编辑网页：只用标准库，不引前端框架。
//
// 页面全部由服务端渲染；只读页面在没有 JavaScript 时也能完整阅读。
package web

import (
	"bytes"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strings"

	"expdeck/internal/core"
)

// assets 同时收录页面模板与静态资源，编译期打包进二进制。
//
//go:embed templates static
var assets embed.FS

// templates 是全部页面模板。模板缺失或语法错误属于启动时就要暴露的问题。
var templates = template.Must(template.ParseFS(assets, "templates/*.html"))

// Server 把 core.Kernel 暴露成网页。
type Server struct {
	kernel *core.Kernel
}

// New 构造一个网页服务。
func New(k *core.Kernel) *Server { return &Server{kernel: k} }

// Handler 返回全部路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	if static, err := fs.Sub(assets, "static"); err == nil {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	} else {
		log.Printf("expdeck web: 读取静态资源失败: %v", err)
	}
	mux.HandleFunc("/", s.handle)
	return mux
}

// handle 按路径分派：/ 是列表，/e/ 之下是详情与编辑。
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/":
		if !onlyGet(w, r) {
			return
		}
		s.handleIndex(w)
	case strings.HasPrefix(r.URL.Path, "/e/"):
		s.handleExperiment(w, r)
	default:
		s.notFound(w)
	}
}

// handleExperiment 处理 /e/ 之下的请求。
//
// 详情页规范地址是 /e/<路径>，编辑页是 /e/<路径>/edit；路径本身以 edit
// 结尾时（如 /e/foo/edit/）靠尾斜杠仍可落到详情页，不会和编辑页撞车。
func (s *Server) handleExperiment(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/e/")
	switch {
	case rest == "":
		s.notFound(w)
	case strings.HasSuffix(rest, "/edit"):
		ref := strings.TrimSuffix(rest, "/edit")
		if ref == "" {
			s.notFound(w)
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.handleEdit(w, ref)
		case http.MethodPost:
			s.handleSave(w, r, ref)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "405 方法不允许", http.StatusMethodNotAllowed)
		}
	default:
		if !onlyGet(w, r) {
			return
		}
		s.handleDetail(w, strings.TrimSuffix(rest, "/"))
	}
}

// handleIndex 渲染实验列表：左侧树、右侧铺平的实验清单。
func (s *Server) handleIndex(w http.ResponseWriter) {
	exps, err := s.kernel.List()
	if err != nil {
		s.internalError(w, err)
		return
	}
	tree := buildTreeView(core.BuildTree(exps), "")
	s.render(w, http.StatusOK, "index", indexData{
		Tree:        tree,
		Experiments: flattenTree(tree),
	})
}

// handleDetail 渲染实验详情。
func (s *Server) handleDetail(w http.ResponseWriter, ref string) {
	exp, ok := s.findExperiment(w, ref)
	if !ok {
		return
	}
	tree, err := s.viewTree(exp.PathString())
	if err != nil {
		s.internalError(w, err)
		return
	}
	note, _, err := s.kernel.ReadNote(exp)
	if err != nil {
		s.internalError(w, err)
		return
	}
	diff, diffErr := s.kernel.BranchDiff(exp)
	worktree, err := s.worktreeOf(exp)
	if err != nil {
		s.internalError(w, err)
		return
	}
	data := detailData{
		Tree:     tree,
		Exp:      newExpView(exp),
		Note:     note,
		Diff:     diff,
		Worktree: worktree,
	}
	if diffErr != nil {
		data.DiffErr = diffErr.Error()
	}
	s.render(w, http.StatusOK, "detail", data)
}

// handleEdit 渲染编辑页，textarea 预填当前笔记。
func (s *Server) handleEdit(w http.ResponseWriter, ref string) {
	exp, ok := s.findExperiment(w, ref)
	if !ok {
		return
	}
	note, version, err := s.kernel.ReadNote(exp)
	if err != nil {
		s.internalError(w, err)
		return
	}
	tree, err := s.viewTree(exp.PathString())
	if err != nil {
		s.internalError(w, err)
		return
	}
	view := newExpView(exp)
	s.render(w, http.StatusOK, "edit", editData{
		Tree:    tree,
		Exp:     view,
		Content: note,
		Version: version,
		Action:  view.EditURL,
	})
}

// handleSave 保存笔记：成功 303 跳详情，版本冲突 409 并重渲染编辑页。
func (s *Server) handleSave(w http.ResponseWriter, r *http.Request, ref string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "400 无法解析提交的表单", http.StatusBadRequest)
		return
	}
	exp, ok := s.findExperiment(w, ref)
	if !ok {
		return
	}
	content := r.PostFormValue("content")
	expect := r.PostFormValue("version")
	if _, err := s.kernel.WriteNote(exp, content, expect); err != nil {
		if errors.Is(err, core.ErrConflict) {
			s.renderConflict(w, exp, content, expect)
			return
		}
		s.internalError(w, err)
		return
	}
	http.Redirect(w, r, newExpView(exp).URL, http.StatusSeeOther)
}

// renderConflict 重新渲染编辑页，保留用户刚提交的内容，状态码 409。
func (s *Server) renderConflict(w http.ResponseWriter, exp *core.Experiment, content, version string) {
	tree, err := s.viewTree(exp.PathString())
	if err != nil {
		s.internalError(w, err)
		return
	}
	view := newExpView(exp)
	s.render(w, http.StatusConflict, "edit", editData{
		Tree:    tree,
		Exp:     view,
		Content: content,
		Version: version,
		Action:  view.EditURL,
		Error:   "笔记已被改动，请重新加载",
	})
}

// findExperiment 按路径或标识找实验；找不到时写 404，内部错误时写 500。
func (s *Server) findExperiment(w http.ResponseWriter, ref string) (*core.Experiment, bool) {
	exp, err := s.kernel.Find(core.NormalizePath(ref))
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			s.notFound(w)
		} else {
			s.internalError(w, err)
		}
		return nil, false
	}
	return exp, true
}

// viewTree 生成左侧树，active 是当前高亮的实验路径。
func (s *Server) viewTree(active string) (*treeNode, error) {
	exps, err := s.kernel.List()
	if err != nil {
		return nil, err
	}
	return buildTreeView(core.BuildTree(exps), active), nil
}

// worktreeOf 返回实验分支当前挂载的 worktree 目录；未挂载时返回空串。
func (s *Server) worktreeOf(exp *core.Experiment) (string, error) {
	worktrees, err := s.kernel.Attached()
	if err != nil {
		return "", err
	}
	for dir, branch := range worktrees {
		if branch == exp.Branch {
			return dir, nil
		}
	}
	return "", nil
}

// render 先渲染到缓冲区，模板出错时还能给出干净的 500。
func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		s.internalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("expdeck web: 写入响应失败: %v", err)
	}
}

func (s *Server) notFound(w http.ResponseWriter) {
	http.Error(w, "404 找不到该实验", http.StatusNotFound)
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	log.Printf("expdeck web: %v", err)
	http.Error(w, "500 服务器内部错误", http.StatusInternalServerError)
}

// onlyGet 只放行 GET/HEAD，其余返回 405。
func onlyGet(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET")
	http.Error(w, "405 只支持 GET", http.StatusMethodNotAllowed)
	return false
}

// ---------- 视图模型 ----------

// expView 是实验在页面上的展示投影。
type expView struct {
	Path        string
	URL         string
	EditURL     string
	Branch      string
	Hypothesis  string
	Status      string
	StatusClass string
}

// treeNode 是左侧树的展示节点；目录节点没有 Exp。
type treeNode struct {
	Name     string
	Exp      *expView
	Active   bool
	Children []*treeNode
}

type indexData struct {
	Tree        *treeNode
	Experiments []*expView
}

type detailData struct {
	Tree     *treeNode
	Exp      *expView
	Note     string
	Diff     string
	DiffErr  string
	Worktree string
}

type editData struct {
	Tree    *treeNode
	Exp     *expView
	Content string
	Version string
	Action  string
	Error   string
}

func newExpView(exp *core.Experiment) *expView {
	return &expView{
		Path:        exp.PathString(),
		URL:         experimentURL(exp.Path),
		EditURL:     editExperimentURL(exp.Path),
		Branch:      exp.Branch,
		Hypothesis:  exp.Hypothesis,
		Status:      exp.Status,
		StatusClass: statusClass(exp.Status),
	}
}

// experimentURL 生成详情页地址：/e/<分段>/<分段>，每段单独转义。
// 末段是 edit 时补一个尾斜杠，否则会和 /e/<路径>/edit 编辑页混淆。
func experimentURL(segs []string) string {
	base := "/e/" + strings.Join(escapeSegs(segs), "/")
	if len(segs) > 0 && segs[len(segs)-1] == "edit" {
		return base + "/"
	}
	return base
}

// editExperimentURL 生成编辑页地址：/e/<分段>/<分段>/edit。
func editExperimentURL(segs []string) string {
	return "/e/" + strings.Join(escapeSegs(segs), "/") + "/edit"
}

// escapeSegs 逐段转义路径，斜杠保持为分隔符。
func escapeSegs(segs []string) []string {
	parts := make([]string, len(segs))
	for i, seg := range segs {
		parts[i] = url.PathEscape(seg)
	}
	return parts
}

// statusClass 把状态映射成状态点的 class：结论绿、放弃红，其余（含进行中）灰。
func statusClass(status string) string {
	switch status {
	case core.StatusConcluded:
		return "concluded"
	case core.StatusAbandoned:
		return "abandoned"
	case core.StatusActive, "running":
		return status
	}
	return "unknown"
}

// buildTreeView 把 core 的树转换成展示节点。
func buildTreeView(node *core.Node, active string) *treeNode {
	view := &treeNode{Name: node.Name}
	for _, child := range node.Children {
		view.Children = append(view.Children, buildTreeView(child, active))
	}
	if node.Exp != nil {
		view.Exp = newExpView(node.Exp)
		view.Active = node.Exp.PathString() == active
	}
	return view
}

// flattenTree 按树的先序把实验铺平成清单。
func flattenTree(node *treeNode) []*expView {
	var out []*expView
	var walk func(n *treeNode)
	walk = func(n *treeNode) {
		if n.Exp != nil {
			out = append(out, n.Exp)
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(node)
	return out
}
