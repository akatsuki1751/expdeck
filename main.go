package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"

	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type record struct {
	Branch     string `json:"branch"`
	Hypothesis string `json:"hypothesis"`
	Notes      string `json:"notes"`
	Updated    string `json:"updated"`
}
type experiment struct{ Branch, Path, Worktree, Hypothesis, Notes string }

var root string

func git(args ...string) (string, error) {
	c := exec.Command("git", args...)
	if root != "" {
		c.Dir = root
	}
	b, e := c.CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}
func discover() error {
	p, e := git("rev-parse", "--show-toplevel")
	if e != nil {
		return errors.New("当前目录不在 Git 仓库中；请先运行 xpd init")
	}
	root = p
	if _, e = os.Stat(filepath.Join(root, "xpd.toml")); e != nil {
		return errors.New("尚未初始化 Expedition；请先运行 xpd init")
	}
	return nil
}
func branch(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "exp/") {
		s = "exp/" + s
	}
	if _, e := git("check-ref-format", "--branch", s); e != nil {
		return "", e
	}
	return s, nil
}
func key(b string) string {
	h := sha256.Sum256([]byte(b))
	return "exps/" + hex.EncodeToString(h[:]) + ".json"
}
func readRecord(b string) record {
	r := record{Branch: b}
	s, e := git("show", "xpd-ledger:"+key(b))
	if e == nil {
		_ = json.Unmarshal([]byte(s), &r)
	}
	return r
}
func save(r record) error {
	r.Updated = time.Now().UTC().Format(time.RFC3339)
	data, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp("", "xpd-index-*")
	if e != nil {
		return e
	}
	idx := f.Name()
	f.Close()
	os.Remove(idx)
	defer os.Remove(idx)
	for i := 0; i < 5; i++ {
		os.Remove(idx)
		old, _ := git("rev-parse", "--verify", "refs/heads/xpd-ledger")
		run := func(input []byte, args ...string) (string, error) {
			c := exec.Command("git", args...)
			c.Dir = root
			c.Env = append(os.Environ(), "GIT_INDEX_FILE="+idx)
			if input != nil {
				c.Stdin = strings.NewReader(string(input))
			}
			out, err := c.CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(out)))
			}
			return strings.TrimSpace(string(out)), nil
		}
		if old != "" {
			if _, e = run(nil, "read-tree", old); e != nil {
				return e
			}
		}
		blob, e := run(data, "hash-object", "-w", "--stdin")
		if e != nil {
			return e
		}
		if _, e = run(nil, "update-index", "--add", "--cacheinfo", "100644,"+blob+","+key(r.Branch)); e != nil {
			return e
		}
		tree, e := run(nil, "write-tree")
		if e != nil {
			return e
		}
		args := []string{"commit-tree", tree, "-m", "Update experiment " + r.Branch}
		if old != "" {
			args = append(args, "-p", old)
		}
		commit, e := run(nil, args...)
		if e != nil {
			return fmt.Errorf("写入台账失败（请检查 Git 用户名和邮箱配置）: %w", e)
		}
		expected := old
		if expected == "" {
			expected = strings.Repeat("0", 40)
		}
		if _, e = git("update-ref", "refs/heads/xpd-ledger", commit, expected); e == nil {
			return nil
		}
	}
	return errors.New("台账同时被修改，请重试")
}
func list() ([]experiment, error) {
	out, e := git("for-each-ref", "--format=%(refname:short)", "refs/heads/exp/")
	if e != nil {
		return nil, e
	}
	wt, e := git("worktree", "list", "--porcelain")
	if e != nil {
		return nil, e
	}
	paths := map[string]string{}
	var p string
	for _, l := range strings.Split(wt, "\n") {
		if strings.HasPrefix(l, "worktree ") {
			p = strings.TrimPrefix(l, "worktree ")
		} else if strings.HasPrefix(l, "branch refs/heads/") {
			paths[strings.TrimPrefix(l, "branch refs/heads/")] = p
		}
	}
	var xs []experiment
	for _, b := range strings.Split(out, "\n") {
		if b == "" {
			continue
		}
		r := readRecord(b)
		xs = append(xs, experiment{b, strings.TrimPrefix(b, "exp/"), paths[b], r.Hypothesis, r.Notes})
	}
	return xs, nil
}
func exists(b string) bool {
	_, e := git("show-ref", "--verify", "--quiet", "refs/heads/"+b)
	return e == nil
}
func attach(b, path string) error {
	if !exists(b) {
		return fmt.Errorf("实验 %s 不存在", b)
	}
	if path == "" {
		path = filepath.Join(filepath.Dir(root), filepath.Base(root)+"-xpd", filepath.FromSlash(strings.TrimPrefix(b, "exp/")))
	}
	path, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	if _, e = git("worktree", "add", path, b); e != nil {
		return e
	}
	fmt.Println("已挂载:", path)
	return nil
}
func detach(b string) error {
	xs, e := list()
	if e != nil {
		return e
	}
	for _, x := range xs {
		if x.Branch == b {
			if x.Worktree == "" {
				return errors.New("实验未挂载")
			}
			if filepath.Clean(x.Worktree) == filepath.Clean(root) {
				return errors.New("不能卸载主工作树")
			}
			if _, e = git("worktree", "remove", x.Worktree); e != nil {
				return e
			}
			fmt.Println("已卸载:", x.Worktree)
			return nil
		}
	}
	return errors.New("实验不存在")
}
func create(b, h string) error {
	if exists(b) {
		return errors.New("实验已存在")
	}
	if _, e := git("rev-parse", "--verify", "HEAD^{commit}"); e != nil {
		return errors.New("当前仓库还没有首次提交。请先用 git add 选择需要纳入实验的文件，再运行 git commit 创建初始提交，然后重试")
	}
	if _, e := git("branch", b, "HEAD"); e != nil {
		return e
	}
	if e := save(record{Branch: b, Hypothesis: h}); e != nil {
		return fmt.Errorf("分支已创建，但记录保存失败: %w", e)
	}
	return nil
}
func initProject(force bool) error {
	if _, e := git("rev-parse", "--show-toplevel"); e != nil {
		if !force {
			return errors.New("当前目录不是 Git 仓库；使用 xpd init --force 创建")
		}
		if _, e = git("init"); e != nil {
			return e
		}
	}
	p, e := git("rev-parse", "--show-toplevel")
	if e != nil {
		return e
	}
	root = p
	f, e := os.OpenFile(filepath.Join(root, "xpd.toml"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if os.IsExist(e) {
		return errors.New("xpd.toml 已存在")
	}
	if e != nil {
		return e
	}
	defer f.Close()
	name := strings.ReplaceAll(filepath.Base(root), "\"", "\\\"")
	_, e = fmt.Fprintf(f, "[project]\nname = \"%s\"\n", name)
	if e == nil {
		fmt.Println("已初始化:", root)
	}
	return e
}
func usage() {
	fmt.Print(`Expedition - 实验管理

  xpd init [--force]                 初始化现有仓库
  xpd new <实验路径> [假设]         创建实验分支
  xpd list                          列出实验
  xpd show <实验路径>               查看假设和笔记
  xpd note <实验路径> <笔记>        更新实验笔记
  xpd attach <实验路径> [目录]      挂载工作树
  xpd detach <实验路径>             卸载工作树（拒绝脏工作区）
  xpd web [--addr 127.0.0.1:8080]  启动本地网页

实验路径可省略 exp/ 前缀；笔记为完整替换。
`)
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, "错误:", e)
		os.Exit(1)
	}
}
func run(a []string) error {
	if len(a) == 0 || a[0] == "help" || a[0] == "--help" {
		usage()
		return nil
	}
	if a[0] == "init" {
		f := flag.NewFlagSet("init", flag.ContinueOnError)
		force := f.Bool("force", false, "")
		if e := f.Parse(a[1:]); e != nil {
			return e
		}
		if f.NArg() != 0 {
			return errors.New("init 参数过多")
		}
		return initProject(*force)
	}
	if e := discover(); e != nil {
		return e
	}
	switch a[0] {
	case "list":
		xs, e := list()
		if e != nil {
			return e
		}
		for _, x := range xs {
			fmt.Printf("%s\t%s\t%s\n", x.Branch, x.Worktree, x.Hypothesis)
		}
		return nil
	case "new", "show", "note", "attach", "detach":
		if len(a) < 2 {
			return errors.New("缺少实验路径")
		}
		b, e := branch(a[1])
		if e != nil {
			return e
		}
		switch a[0] {
		case "new":
			if len(a) > 3 {
				return errors.New("参数过多")
			}
			h := ""
			if len(a) == 3 {
				h = a[2]
			}
			if e = create(b, h); e != nil {
				return e
			}
			fmt.Println("已创建:", b)
			return nil
		case "show":
			if len(a) != 2 {
				return errors.New("参数过多")
			}
			if !exists(b) {
				return errors.New("实验不存在")
			}
			r := readRecord(b)
			fmt.Printf("实验: %s\n假设: %s\n笔记:\n%s\n", b, r.Hypothesis, r.Notes)
			return nil
		case "note":
			if len(a) != 3 {
				return errors.New("请提供完整笔记内容")
			}
			if !exists(b) {
				return errors.New("实验不存在")
			}
			r := readRecord(b)
			r.Notes = a[2]
			return save(r)
		case "attach":
			if len(a) > 3 {
				return errors.New("参数过多")
			}
			p := ""
			if len(a) == 3 {
				p = a[2]
			}
			return attach(b, p)
		case "detach":
			if len(a) != 2 {
				return errors.New("参数过多")
			}
			return detach(b)
		}
	case "web":
		f := flag.NewFlagSet("web", flag.ContinueOnError)
		addr := f.String("addr", "127.0.0.1:8080", "")
		if e := f.Parse(a[1:]); e != nil {
			return e
		}
		return serve(*addr)
	}
	return errors.New("未知命令；运行 xpd help 查看用法")
}
