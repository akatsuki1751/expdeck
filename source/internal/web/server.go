// Package web 是 ExpDeck 的浏览与笔记编辑网页：Go 服务端渲染，HTMX 增强交互。
//
// 页面全部由服务端渲染；只读页面在没有 JavaScript 时也能完整阅读。
package web

import (
	"bytes"
	"crypto/subtle"
	"embed"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
	// Token 非空时，所有请求都要带令牌：先用 ?token=... 打开一次换 cookie。
	Token string
}

// New 构造一个网页服务。
func New(k *core.Kernel) *Server { return &Server{kernel: k} }

const tokenCookie = "exp_token"

// Handler 返回全部路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	if static, err := fs.Sub(assets, "static"); err == nil {
		staticFiles := http.StripPrefix("/static/", http.FileServer(http.FS(static)))
		mux.Handle("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The embedded assets can change whenever the binary is rebuilt.
			w.Header().Set("Cache-Control", "no-cache")
			staticFiles.ServeHTTP(w, r)
		}))
	} else {
		log.Printf("expdeck web: 读取静态资源失败: %v", err)
	}
	mux.HandleFunc("/", s.handle)
	return s.guard(s.actionLog(mux))
}

// guard 做两件事：令牌校验，以及拒绝跨站写请求。
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 跨站表单提交：Origin 与 Host 不一致就拒。GET 不受影响。
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || parsed.Host != r.Host {
					http.Error(w, "跨站请求被拒绝", http.StatusForbidden)
					return
				}
			}
		}
		if s.Token == "" {
			next.ServeHTTP(w, r)
			return
		}
		if supplied := r.URL.Query().Get("token"); supplied != "" {
			if subtle.ConstantTimeCompare([]byte(supplied), []byte(s.Token)) != 1 {
				http.Error(w, "令牌无效", http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     tokenCookie,
				Value:    s.Token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
			target := *r.URL
			query := target.Query()
			query.Del("token")
			target.RawQuery = query.Encode()
			http.Redirect(w, r, target.String(), http.StatusSeeOther)
			return
		}
		if cookie, err := r.Cookie(tokenCookie); err == nil {
			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.Token)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `<!doctype html><meta charset="utf-8"><title>ExpDeck</title>`+
			`<p>需要令牌：请用 <code>exp serve</code> 打印的带 token 的链接打开一次。</p>`)
	})
}

// handle 按路径分派：/ 是列表，/e/ 之下是详情与编辑。
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	// Boosted forms follow redirects; keep the visible URL aligned with the rendered page.
	if r.Method == http.MethodGet && r.Header.Get("HX-Boosted") == "true" {
		w.Header().Set("HX-Push-Url", r.URL.RequestURI())
	}
	if r.Method == http.MethodPost {
		switch r.URL.Path {
		case "/new":
			s.handleNew(w, r)
			return
		case "/adopt":
			s.handleAdopt(w, r)
			return
		case "/backup":
			s.handleBackup(w, r)
			return
		case "/restore":
			s.handleRestore(w, r)
			return
		case "/gc":
			s.handleGC(w, r)
			return
		}
	}
	switch {
	case r.URL.Path == "/":
		if !onlyGet(w, r) {
			return
		}
		s.handleIndex(w, r)
	case strings.HasPrefix(r.URL.Path, "/e/"):
		s.handleExperiment(w, r)
	case r.URL.Path == "/activity":
		s.handleActivity(w, r)
	case strings.HasPrefix(r.URL.Path, "/log/"):
		if !onlyGet(w, r) {
			return
		}
		s.handleLog(w, r)
	default:
		s.notFound(w)
	}
}

// handleExperiment 处理 /e/ 之下的请求。
//
// 详情页规范地址是 /e/<路径>，编辑页是 /e/<路径>/edit，运行动作是
// /e/<路径>/run 与 /e/<路径>/interrupt。路径本身以 edit 结尾时（如
// /e/foo/edit/）靠尾斜杠仍可落到详情页，不会和编辑页撞车；以 run 或
// interrupt 结尾的路径，GET 会按详情页处理，只有 POST 才是动作。
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
	case strings.HasSuffix(rest, "/run"):
		ref := strings.TrimSuffix(rest, "/run")
		if ref == "" {
			s.notFound(w)
			return
		}
		switch r.Method {
		case http.MethodPost:
			s.handleRun(w, r, ref)
		case http.MethodGet, http.MethodHead:
			s.handleDetail(w, r, strings.TrimSuffix(rest, "/"))
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "405 方法不允许", http.StatusMethodNotAllowed)
		}
	case strings.HasSuffix(rest, "/interrupt"):
		ref := strings.TrimSuffix(rest, "/interrupt")
		if ref == "" {
			s.notFound(w)
			return
		}
		switch r.Method {
		case http.MethodPost:
			s.handleInterrupt(w, r, ref)
		case http.MethodGet, http.MethodHead:
			s.handleDetail(w, r, strings.TrimSuffix(rest, "/"))
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "405 方法不允许", http.StatusMethodNotAllowed)
		}
	default:
		if action, ref, ok := splitAction(rest); ok && r.Method == http.MethodPost {
			s.handleExpAction(w, r, ref, action)
			return
		}
		if !onlyGet(w, r) {
			return
		}
		s.handleDetail(w, r, strings.TrimSuffix(rest, "/"))
	}
}

// handleIndex 渲染实验列表：左侧树、右侧铺平的实验清单。
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	exps, err := s.kernel.List()
	if err != nil {
		s.internalError(w, err)
		return
	}
	unregistered, err := s.kernel.Unregistered()
	if err != nil {
		s.internalError(w, err)
		return
	}
	tree := buildTreeView(core.BuildTree(exps), "")
	if err := s.populateTree(tree, ""); err != nil {
		s.internalError(w, err)
		return
	}
	s.render(w, http.StatusOK, "index", indexData{
		Project:      s.projectName(),
		Tree:         tree,
		Experiments:  flattenTree(tree),
		Unregistered: unregistered,
		Error:        r.URL.Query().Get("error"),
		Notice:       r.URL.Query().Get("ok"),
	})
}

// handleDetail 渲染实验详情。
func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request, ref string) {
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
		Project:   s.projectName(),
		Tree:      tree,
		Exp:       newExpView(exp),
		Note:      note,
		Diff:      diff,
		DiffLines: parseDiff(diff),
		Worktree:  worktree,
		Attached:  worktree != "",
		Error:     r.URL.Query().Get("error"),
	}
	if diffErr != nil {
		data.DiffErr = diffErr.Error()
	}
	if selected := r.URL.Query().Get("run"); selected != "" {
		runs, err := s.kernel.ListRuns(exp)
		if err != nil {
			s.internalError(w, err)
			return
		}
		for _, run := range runs {
			if run.ID == selected {
				data.SelectedRun = run
				break
			}
		}
		if data.SelectedRun == nil {
			s.notFound(w)
			return
		}
		s.kernel.Reconcile(data.SelectedRun)
		data.SelectedLog, _, data.SelectedLogTruncated = readLogTail(s.kernel.LogPath(data.SelectedRun.ID))
		markSelectedRun(tree, exp.PathString(), selected)
	}
	running, err := s.kernel.RunningRun(exp)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if running != nil {
		data.RunID = running.ID
		data.LogText, data.LogOffset, data.LogTruncated = readLogTail(running.Log)
	}
	s.render(w, http.StatusOK, "detail", data)
}

// handleRun 起一次运行：成功跳回详情页，失败也跳回，只是带上错误提示。
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request, ref string) {
	exp, ok := s.findExperiment(w, ref)
	if !ok {
		return
	}
	if _, err := s.kernel.StartRun(exp, 0); err != nil {
		s.redirectError(w, r, exp, err)
		return
	}
	http.Redirect(w, r, newExpView(exp).URL, http.StatusSeeOther)
}

// handleInterrupt 中断该实验当前正在跑的 run；没有在跑时带错误提示跳回。
func (s *Server) handleInterrupt(w http.ResponseWriter, r *http.Request, ref string) {
	exp, ok := s.findExperiment(w, ref)
	if !ok {
		return
	}
	running, err := s.kernel.RunningRun(exp)
	if err != nil {
		s.redirectError(w, r, exp, err)
		return
	}
	if running == nil {
		s.redirectError(w, r, exp, errors.New("没有正在运行的 run"))
		return
	}
	if _, err := s.kernel.InterruptRun(running.ID); err != nil {
		s.redirectError(w, r, exp, err)
		return
	}
	http.Redirect(w, r, newExpView(exp).URL, http.StatusSeeOther)
}

// redirectError 把失败原因放进查询串，跳回详情页展示。
func (s *Server) redirectError(w http.ResponseWriter, r *http.Request, exp *core.Experiment, err error) {
	target := newExpView(exp).URL + "?error=" + url.QueryEscape(err.Error())
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// logTailLimit 是详情页预读日志的上限：只读末尾，避免把整份训练日志塞进页面。
const logTailLimit = 64 << 10

// readLogTail 读出日志末尾一段，并返回已读到末尾时的字节偏移。
//
// offset 是「本次已经读到文件第几字节」，供页面上的增量轮询接着读。
func readLogTail(path string) (text string, offset int64, truncated bool) {
	if path == "" {
		return "", 0, false
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return "", 0, false
	}
	start := int64(0)
	if info.Size() > logTailLimit {
		start = info.Size() - logTailLimit
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return "", info.Size(), false
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return "", start, false
	}
	offset = start + int64(len(data))
	cut := 0
	if start > 0 {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			cut = i + 1
		}
	}
	return string(data[cut:]), offset, start > 0
}

// handleLog 返回日志从 offset 字节开始的增量内容。
//
// 文件或 run 不存在时返回 200 加空正文，页面轮询时不会因此报错。
func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	runID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/log/"), "/")
	if runID == "" || strings.Contains(runID, "/") {
		return
	}
	_, run, err := s.kernel.FindRun(runID)
	if err != nil || run.Log == "" {
		return
	}
	offset := int64(0)
	if text := r.URL.Query().Get("offset"); text != "" {
		if value, convErr := strconv.ParseInt(text, 10, 64); convErr == nil && value > 0 {
			offset = value
		}
	}
	file, err := os.Open(run.Log)
	if err != nil {
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return
	}
	if offset > info.Size() {
		offset = info.Size()
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return
	}
	if _, err := io.Copy(w, file); err != nil {
		log.Printf("expdeck web: 读取日志失败: %v", err)
	}
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
		Project: s.projectName(),
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
	tree := buildTreeView(core.BuildTree(exps), active)
	if err := s.populateTree(tree, ""); err != nil {
		return nil, err
	}
	return tree, nil
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
	ID           string
	Path         string
	URL          string
	EditURL      string
	RunURL       string
	InterruptURL string
	AttachURL    string
	DetachURL    string
	ConcludeURL  string
	ReopenURL    string
	RenameURL    string
	Attached     bool
	Branch       string
	Hypothesis   string
	Status       string
	StatusClass  string
}

// treeNode 是左侧树的展示节点；目录节点没有 Exp。
type runView struct {
	ID, URL, Status string
	Active          bool
}
type treeNode struct {
	Runs     []runView
	Path     string
	Name     string
	Exp      *expView
	Active   bool
	Children []*treeNode
}

type indexData struct {
	Project      string
	Tree         *treeNode
	Experiments  []*expView
	Unregistered []string
	Error        string
	Notice       string
}

type detailData struct {
	SelectedRun          *core.Run
	SelectedLog          string
	SelectedLogTruncated bool
	Project              string
	Tree                 *treeNode
	Exp                  *expView
	Note                 string
	Diff                 string
	DiffLines            []diffLine
	DiffErr              string
	Worktree             string
	Attached             bool
	Error                string
	RunID                string
	LogText              string
	LogOffset            int64
	LogTruncated         bool
}

type editData struct {
	Project string
	Tree    *treeNode
	Exp     *expView
	Content string
	Version string
	Action  string
	Error   string
}

// projectName 是仓库目录名，展示在左栏顶部。
func (s *Server) projectName() string {
	if s.kernel == nil || s.kernel.Repo == nil {
		return "ExpDeck"
	}
	return filepath.Base(s.kernel.Repo.Root)
}

func newExpView(exp *core.Experiment) *expView {
	return &expView{
		ID:           exp.ID,
		Path:         exp.PathString(),
		URL:          experimentURL(exp.Path),
		EditURL:      editExperimentURL(exp.Path),
		RunURL:       experimentActionURL(exp.Path, "run"),
		InterruptURL: experimentActionURL(exp.Path, "interrupt"),
		AttachURL:    experimentActionURL(exp.Path, "attach"),
		DetachURL:    experimentActionURL(exp.Path, "detach"),
		ConcludeURL:  experimentActionURL(exp.Path, "conclude"),
		ReopenURL:    experimentActionURL(exp.Path, "reopen"),
		RenameURL:    experimentActionURL(exp.Path, "rename"),
		Branch:       exp.Branch,
		Hypothesis:   exp.Hypothesis,
		Status:       exp.Status,
		StatusClass:  statusClass(exp.Status),
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

// experimentActionURL 生成运行动作地址：/e/<分段>/<分段>/<动作>。
func experimentActionURL(segs []string, action string) string {
	return "/e/" + strings.Join(escapeSegs(segs), "/") + "/" + action
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
	view := &treeNode{Name: node.Name, Path: strings.Join(node.Path, "/")}
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

// populateTree loads the attached state and the runs shown under each experiment.
func (s *Server) populateTree(tree *treeNode, activeRun string) error {
	worktrees, err := s.kernel.Attached()
	if err != nil {
		return err
	}
	branches := map[string]bool{}
	for _, branch := range worktrees {
		branches[branch] = true
	}
	var walk func(*treeNode) error
	walk = func(n *treeNode) error {
		if n.Exp != nil {
			n.Exp.Attached = branches[n.Exp.Branch]
			exp := &core.Experiment{ID: n.Exp.ID}
			runs, err := s.kernel.ListRuns(exp)
			if err != nil {
				return err
			}
			for i := len(runs) - 1; i >= 0; i-- {
				run := runs[i]
				n.Runs = append(n.Runs, runView{ID: run.ID, Status: run.Status, URL: n.Exp.URL + "?run=" + url.QueryEscape(run.ID), Active: n.Active && run.ID == activeRun})
			}
		}
		for _, child := range n.Children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(tree)
}
func markSelectedRun(tree *treeNode, path, runID string) {
	if tree.Exp != nil && tree.Exp.Path == path {
		for i := range tree.Runs {
			tree.Runs[i].Active = tree.Runs[i].ID == runID
		}
		tree.Active = false
	}
	for _, child := range tree.Children {
		markSelectedRun(child, path, runID)
	}
}
