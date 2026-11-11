// cmd_note.go 提供 note / open 两个命令：读、写、追加实验笔记。

package cli

import (
	"fmt"
	"io"
	"strings"

	"expdeck/internal/core"
)

// noteUnset 是 --text / --append 的初始值：splitArgs 只回填值，不报告是否出现过。
const noteUnset = "\x00unset"

// cmdNote 读写实验笔记：--print、--text、--append 三选一。
func (a *app) cmdNote(args []string) (result, error) {
	const usage = "usage: exp note <path|id> [--print | --text <text> | --append <text>] [--expect <version>]"
	var text, add, expect string
	text, add = noteUnset, noteUnset
	var printNote bool
	pos, err := splitArgs(args,
		map[string]*string{"text": &text, "append": &add, "expect": &expect},
		map[string]*bool{"print": &printNote},
	)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	hasText := text != noteUnset
	hasAppend := add != noteUnset
	if !hasText {
		text = ""
	}
	if !hasAppend {
		add = ""
	}
	if len(pos) != 1 {
		return result{}, fail(ExitUsage, "%s", usage)
	}
	given := 0
	for _, set := range []bool{hasText, hasAppend, printNote} {
		if set {
			given++
		}
	}
	if given != 1 {
		return result{}, fail(ExitUsage, "%s", usage)
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	exp, err := kernel.Find(pos[0])
	if err != nil {
		return result{}, err
	}
	notePath := kernel.NotePath(exp)

	if printNote {
		content, version, readErr := kernel.ReadNote(exp)
		if readErr != nil {
			return result{}, readErr
		}
		return result{
			data: map[string]any{"content": content, "version": version},
			human: func(w io.Writer) {
				if content == "" {
					fmt.Fprintf(w, "(empty note)  %s\n", notePath)
					return
				}
				fmt.Fprint(w, content)
				if !strings.HasSuffix(content, "\n") {
					fmt.Fprintln(w)
				}
				fmt.Fprintf(w, "version  %s\n", version)
			},
		}, nil
	}

	if a.dry {
		plan := map[string]any{
			"dry_run":    true,
			"experiment": exp.PathString(),
			"note":       notePath,
		}
		if hasText {
			plan["mode"] = "replace"
			plan["bytes"] = len(text)
		} else {
			plan["mode"] = "append"
			plan["bytes"] = len(add)
		}
		if expect != "" {
			plan["expect"] = expect
		}
		return result{data: plan}, nil
	}

	var version string
	if hasText {
		version, err = kernel.WriteNote(exp, text, expect)
	} else {
		version, err = kernel.AppendNote(exp, add)
	}
	if err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{"experiment": exp.PathString(), "version": version},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "wrote  %s  (%s)\n", notePath, version)
		},
	}, nil
}

// cmdOpen 返回笔记路径与内容，不启动编辑器（供网页/编辑器使用）。
func (a *app) cmdOpen(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) != 1 {
		return result{}, fail(ExitUsage, "usage: exp open <path|id>")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	exp, err := kernel.Find(pos[0])
	if err != nil {
		return result{}, err
	}
	notePath := kernel.NotePath(exp)
	content, version, err := kernel.ReadNote(exp)
	if err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{
			"experiment": exp.PathString(),
			"note":       notePath,
			"content":    content,
			"version":    version,
		},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "note     %s\n", notePath)
			fmt.Fprintf(w, "version  %s\n", version)
			if content == "" {
				return
			}
			fmt.Fprintln(w)
			fmt.Fprint(w, content)
			if !strings.HasSuffix(content, "\n") {
				fmt.Fprintln(w)
			}
		},
	}, nil
}
