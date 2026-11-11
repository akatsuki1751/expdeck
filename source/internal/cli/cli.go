// Package cli 是命令行前端：解析、输出、退出码。
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"expdeck/internal/core"
)

// Env 是一次调用的环境，测试里可以任意替换。
type Env struct {
	Dir    string
	Stdout io.Writer
	Stderr io.Writer
	IsTTY  bool
}

// 退出码契约。
const (
	ExitOK       = 0
	ExitError    = 1
	ExitUsage    = 2
	ExitNotFound = 3
	ExitInvalid  = 4
	ExitConflict = 5
)

type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

func fail(code int, format string, args ...any) error {
	return &exitError{code: code, msg: fmt.Sprintf(format, args...)}
}

type result struct {
	data  any
	human func(w io.Writer)
}

type app struct {
	env  Env
	json bool
	dry  bool
}

// Run 执行一次命令并返回进程退出码。
func Run(env Env, args []string) int {
	if env.Dir == "" {
		env.Dir = "."
	}
	if env.Stdout == nil {
		env.Stdout = io.Discard
	}
	if env.Stderr == nil {
		env.Stderr = io.Discard
	}
	a := &app{env: env, json: !env.IsTTY}
	rest, err := a.parseGlobals(args)
	if err != nil {
		return a.finish(result{}, err)
	}
	if len(rest) == 0 {
		return a.finish(result{}, fail(ExitUsage, "usage: exp <init|new|ls|show> [flags]"))
	}
	cmd, rest := rest[0], rest[1:]
	var res result
	switch cmd {
	case "init":
		res, err = a.cmdInit(rest)
	case "new":
		res, err = a.cmdNew(rest)
	case "ls":
		res, err = a.cmdList(rest)
	case "show":
		res, err = a.cmdShow(rest)
	case "note":
		res, err = a.cmdNote(rest)
	case "open":
		res, err = a.cmdOpen(rest)
	case "conclude":
		res, err = a.cmdConclude(rest)
	case "reopen":
		res, err = a.cmdReopen(rest)
	case "attach":
		res, err = a.cmdAttach(rest)
	case "detach":
		res, err = a.cmdDetach(rest)
	case "backup":
		res, err = a.cmdBackup(rest)
	case "restore":
		res, err = a.cmdRestore(rest)
	case "serve":
		res, err = a.cmdServe(rest)
	case "run":
		res, err = a.cmdRun(rest)
	case "status":
		res, err = a.cmdStatus(rest)
	case "wait":
		res, err = a.cmdWait(rest)
	case "interrupt":
		res, err = a.cmdInterrupt(rest)
	case "__watch":
		res, err = a.cmdWatch(rest)
	default:
		err = fail(ExitUsage, "unknown command %q", cmd)
	}
	return a.finish(res, err)
}

func (a *app) parseGlobals(args []string) ([]string, error) {
	var rest []string
	for _, arg := range args {
		switch arg {
		case "--json":
			a.json = true
		case "--dry-run":
			a.dry = true
		case "-h", "--help":
			return nil, fail(ExitUsage, "usage: exp <init|new|ls|show> [--json] [--dry-run]")
		default:
			rest = append(rest, arg)
		}
	}
	return rest, nil
}

func (a *app) finish(res result, err error) int {
	if err != nil {
		code := codeOf(err)
		if a.json {
			payload := map[string]any{
				"ok": false,
				"error": map[string]any{
					"code":    codeName(code),
					"message": err.Error(),
				},
			}
			_ = json.NewEncoder(a.env.Stdout).Encode(payload)
		} else {
			fmt.Fprintf(a.env.Stderr, "error: %v\n", err)
		}
		return code
	}
	if a.json {
		payload := map[string]any{"ok": true, "data": res.data}
		enc := json.NewEncoder(a.env.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(payload)
		return ExitOK
	}
	if res.human != nil {
		res.human(a.env.Stdout)
	}
	return ExitOK
}

func codeOf(err error) int {
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	switch {
	case errors.Is(err, core.ErrNotFound):
		return ExitNotFound
	case errors.Is(err, core.ErrConflict):
		return ExitConflict
	case errors.Is(err, core.ErrInvalid):
		return ExitInvalid
	}
	return ExitError
}

func codeName(code int) string {
	switch code {
	case ExitUsage:
		return "usage"
	case ExitNotFound:
		return "not_found"
	case ExitInvalid:
		return "invalid"
	case ExitConflict:
		return "conflict"
	case ExitOK:
		return "ok"
	}
	return "error"
}

// splitArgs 允许 flag 出现在位置参数之后（`exp new a/b --why x`）。
func splitArgs(args []string, strFlags map[string]*string, boolFlags map[string]*bool) ([]string, error) {
	var pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			pos = append(pos, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		value := ""
		hasValue := false
		if idx := strings.Index(name, "="); idx >= 0 {
			value, name, hasValue = name[idx+1:], name[:idx], true
		}
		if target, ok := strFlags[name]; ok {
			if !hasValue {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("flag --%s needs a value", name)
				}
				i++
				value = args[i]
			}
			*target = value
			continue
		}
		if target, ok := boolFlags[name]; ok {
			if hasValue {
				return nil, fmt.Errorf("flag --%s takes no value", name)
			}
			*target = true
			continue
		}
		return nil, fmt.Errorf("unknown flag %q", arg)
	}
	return pos, nil
}

// ---------- init ----------

const agentsMarker = "<!-- exp:agent-instructions -->"

func (a *app) cmdInit(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) > 0 {
		return result{}, fail(ExitUsage, "init takes no arguments")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	tomlPath := filepath.Join(kernel.Repo.Root, "exp.toml")
	agentsPath := filepath.Join(kernel.Repo.Root, "AGENTS.md")
	payload := map[string]any{
		"root":    kernel.Repo.Root,
		"records": kernel.Store.Records(),
		"toml":    tomlPath,
		"agents":  agentsPath,
	}
	human := func(w io.Writer) {
		fmt.Fprintf(w, "repository  %s\n", kernel.Repo.Root)
		fmt.Fprintf(w, "records     %s\n", kernel.Store.Records())
		fmt.Fprintf(w, "config      %s\n", tomlPath)
	}
	if a.dry {
		payload["dry_run"] = true
		humanOut := human
		return result{data: payload, human: humanOut}, nil
	}
	if err := kernel.Init(); err != nil {
		return result{}, err
	}
	if err := writeIfAbsent(tomlPath, defaultToml(filepath.Base(kernel.Repo.Root))); err != nil {
		return result{}, err
	}
	if err := appendAgentsBlock(agentsPath); err != nil {
		return result{}, err
	}
	return result{data: payload, human: human}, nil
}

func writeIfAbsent(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func defaultToml(name string) string {
	return fmt.Sprintf(`# ExpDeck 项目配置
[project]
name = %q

[tree]
prefix = "exp"

[store]
path = ""

[run]
cmd = ""
cwd = "."
timeout = "12h"

[metrics]
file = ""
primary = ""
direction = "max"

[artifacts]
glob = []
keep = "best"
`, name)
}

func appendAgentsBlock(path string) error {
	const block = agentsMarker + `
## ExpDeck

实验记录用 ` + "`exp`" + ` 管理，不要直接操作 git 的 worktree 或手写记录文件。

- ` + "`exp init`" + ` 接入项目（已做过）
- ` + "`exp new <path> --why \"...\"`" + ` 新建实验，` + "`--why`" + ` 必填
- ` + "`exp ls`" + ` 列出实验树
- ` + "`exp show <path>`" + ` 查看实验
- 所有命令支持 ` + "`--json`" + `；写操作支持 ` + "`--dry-run`" + `
`
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(existing), agentsMarker) {
		return nil
	}
	out := string(existing)
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return os.WriteFile(path, []byte(out+block), 0o644)
}

// ---------- new ----------

func (a *app) cmdNew(args []string) (result, error) {
	why := ""
	pos, err := splitArgs(args, map[string]*string{"why": &why}, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) != 1 {
		return result{}, fail(ExitUsage, "usage: exp new <path> --why \"...\"")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	pathArg := pos[0]
	if a.dry {
		return result{data: map[string]any{"dry_run": true, "path": pathArg, "why": why}}, nil
	}
	exp, err := kernel.Create(pathArg, why)
	if err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{"experiment": exp},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "created  %s  (%s)\n", exp.PathString(), exp.Branch)
		},
	}, nil
}

// ---------- ls ----------

func (a *app) cmdList(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) > 0 {
		return result{}, fail(ExitUsage, "ls takes no arguments")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	exps, err := kernel.List()
	if err != nil {
		return result{}, err
	}
	tree := core.BuildTree(exps)
	return result{
		data: map[string]any{"experiments": exps},
		human: func(w io.Writer) {
			if len(exps) == 0 {
				fmt.Fprintln(w, "(no experiments)")
				return
			}
			renderNode(w, tree, 0)
		},
	}, nil
}

func renderNode(w io.Writer, node *core.Node, depth int) {
	for _, child := range node.Children {
		indent := strings.Repeat("  ", depth)
		label := indent + child.Name
		if child.Exp != nil {
			fmt.Fprintf(w, "%s  [%s]  %s\n", label, child.Exp.Status, child.Exp.Hypothesis)
			continue
		}
		fmt.Fprintf(w, "%s/\n", label)
		renderNode(w, child, depth+1)
	}
}

// ---------- show ----------

func (a *app) cmdShow(args []string) (result, error) {
	showDiff := false
	pos, err := splitArgs(args, nil, map[string]*bool{"diff": &showDiff})
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) != 1 {
		return result{}, fail(ExitUsage, "usage: exp show <path|id> [--diff]")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}

	if exp, findErr := kernel.Find(pos[0]); findErr == nil {
		runs, runErr := kernel.ListRuns(exp)
		if runErr != nil {
			return result{}, runErr
		}
		return result{
			data: map[string]any{"experiment": exp, "runs": runs},
			human: func(w io.Writer) {
				fmt.Fprintf(w, "%s\n", exp.PathString())
				fmt.Fprintf(w, "  branch      %s\n", exp.Branch)
				fmt.Fprintf(w, "  status      %s\n", exp.Status)
				fmt.Fprintf(w, "  hypothesis  %s\n", exp.Hypothesis)
				fmt.Fprintf(w, "  id          %s\n", exp.ID)
				for _, run := range runs {
					fmt.Fprintf(w, "  run         %s  %s\n", run.ID, run.Status)
				}
			},
		}, nil
	}

	exp, run, runErr := kernel.FindRun(pos[0])
	if runErr != nil {
		return result{}, runErr
	}
	payload := map[string]any{"run": run, "experiment": exp.PathString()}
	diff := ""
	if showDiff {
		diff, err = kernel.RunDiff(run)
		if err != nil {
			return result{}, fail(ExitConflict, "%v", err)
		}
		payload["diff"] = diff
	}
	return result{
		data: payload,
		human: func(w io.Writer) {
			fmt.Fprintf(w, "run       %s\n", run.ID)
			fmt.Fprintf(w, "exp       %s\n", exp.PathString())
			fmt.Fprintf(w, "status    %s\n", run.Status)
			fmt.Fprintf(w, "command   %s\n", run.Command)
			if run.Snapshot != "" {
				fmt.Fprintf(w, "snapshot  %s\n", run.Snapshot)
			}
			if run.ExitCode != nil {
				fmt.Fprintf(w, "exit      %d\n", *run.ExitCode)
			}
			if showDiff {
				fmt.Fprintf(w, "\n%s", diff)
			}
		},
	}, nil
}
