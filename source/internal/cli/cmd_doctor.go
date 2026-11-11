// cmd_doctor.go 提供 doctor 命令：报告记录与仓库之间的漂移。

package cli

import (
	"fmt"
	"io"

	"expdeck/internal/core"
)

// cmdDoctor 体检并列出漂移；发现漂移不算错误，退出码仍为 0。
func (a *app) cmdDoctor(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) > 0 {
		return result{}, fail(ExitUsage, "doctor takes no arguments")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	drifts, err := kernel.Doctor()
	if err != nil {
		return result{}, err
	}
	if drifts == nil {
		drifts = []core.Drift{}
	}
	return result{
		data: map[string]any{"drifts": drifts},
		human: func(w io.Writer) {
			if len(drifts) == 0 {
				fmt.Fprintln(w, "no drift")
				return
			}
			for _, drift := range drifts {
				fmt.Fprintf(w, "%s  %s  %s\n", drift.Kind, drift.Subject, drift.Detail)
			}
		},
	}, nil
}
