package cli

import (
	"fmt"
	"io"
)

// printHelp 打印完整用法。
func printHelp(w io.Writer) {
	fmt.Fprint(w, helpText)
}

func (a *app) cmdVersion(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) > 0 {
		return result{}, fail(ExitUsage, "version takes no arguments")
	}
	return result{
		data: map[string]any{"version": version},
		human: func(w io.Writer) {
			fmt.Fprintln(w, version)
		},
	}, nil
}

const helpText = `ExpDeck — 用 git 分支管理实验，用普通文件记录假设与结论。

用法
  exp <command> [flags]
  exp help

接入
  init                          接入当前 git 仓库：建数据根、写 exp.toml 与 AGENTS.md

实验
  new <path> --why "..."        新建实验（分支 exp/<path> + 记录），--why 必填
  ls                            实验树
  show <exp|run> [--diff]       查看实验，或某个 run；--diff 显示该 run 自己的改动
  attach <exp> [dir]            把分支检出成工作树
  detach <exp>                  收回工作树（未提交改动会固化成 patch）
  adopt <branch> [--why "..."]  把已经存在、但没登记的 exp/ 分支收编成实验

记录
  note <exp> --text "..."       替换整篇笔记
  note <exp> --append "..."     追加一段（带时间戳）
  note <exp> --print            打印笔记与版本号
  open <exp>                    打印笔记路径与内容
  conclude <exp> --decision keep|abandon --why "..."
  reopen <exp>                  重新打开已结案的实验

运行
  run <exp> [--gpu N]           起一次运行，立刻返回 run 标识
  wait --run <id> [--timeout 秒]  阻塞到结束
  status                        正在跑的实验
  interrupt --run <id>          中断运行

维护
  backup                        把记录提交成 book 分支
  restore [--force]             从 book 恢复记录
  gc                            回收没人引用的产物
  doctor                        报告漂移（孤儿实验、未登记分支、失效快照）

网页
  serve [--addr 127.0.0.1:8787] 启动网页，打印带一次性令牌的地址

全局 flag
  --json      结构化输出；标准输出不是终端时自动开启
  --dry-run   只说不做

退出码
  0 成功 · 1 通用错误 · 2 用法错误 · 3 不存在 · 4 校验失败 · 5 冲突
`
