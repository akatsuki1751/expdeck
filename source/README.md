# ExpDeck

用 git 分支管理实验，用普通文件记录假设与结论。agent 用命令行，人用浏览器。

## 构建

```sh
go build -o exp ./cmd/exp
```

单二进制，Go 后端只使用标准库和 `git` 命令行。网页使用随程序内嵌的 HTMX，不需要联网加载前端资源。

```sh
exp --help          # 全部命令
exp version         # 构建时注入的 commit
```

## 快速开始

```sh
cd 你的仓库            # 必须已经是一个 git 仓库，且至少有一次提交
exp init               # 建数据根、写 exp.toml、追加 AGENTS.md 说明
exp new lr/1e-4 --why "lr 降到 1e-4 能提升 r2"
exp attach lr/1e-4     # 检出到独立目录
exp run lr/1e-4        # 立刻返回 run 标识
exp wait --run <id>    # 阻塞到结束
exp show lr/1e-4       # 看假设、状态与全部 run
exp note lr/1e-4 --append "第 3 个 epoch 开始过拟合"
exp conclude lr/1e-4 --decision abandon --why "验证集过拟合"
exp serve              # 只监听本机，启动时打印带令牌的地址
```

网页静态素材保存在 `source/internal/web/static/`：`icons/` 存放 Lucide SVG 与许可说明，`brand/` 存放项目自己的浏览器图标；它们随 Go 二进制一起内嵌。

## 网页的访问控制

`exp serve` 每次启动生成一次性令牌，打印形如 `http://127.0.0.1:8787/?token=…` 的地址。用带 token 的地址打开一次，服务端换成 `HttpOnly` + `SameSite=Lax` 的 cookie，之后正常访问。

- 没有令牌：401
- 跨站 POST（Origin 与 Host 不一致）：403
- 端口转发出去时，令牌就是唯一的一道门

训练超时由 `exp.toml` 的 `[run] timeout` 控制（例如 `"12h"`），到点先发 `SIGTERM`，十秒后 `SIGKILL`，状态记为 `timedout`。

## 网页工作区

实验树中的菜单支持查看、编辑笔记、重命名和挂载/卸载。重命名会更新分支和实验记录；已有工作树保留原目录。左右侧栏可以拖动调整宽度或收起，设置中的浅色/深色主题和侧栏布局保存在当前浏览器。

设置中可以查看网页操作日志。日志位于 Git 公共目录下的 `exp/logs/web.log`，记录操作路径、时间、结果与失败原因；不会记录笔记正文或访问令牌，也不会包含在 `exp backup` 中。

## 数据在哪

```
<仓库>/.git/exp/
├── records/            实验、笔记、run（唯一真相）
├── objects/<xx>/<sha>  产物，按内容哈希
├── logs/<run>.log      训练日志（不进备份）
└── registry/           谁在跑（缓存，可重建）
<仓库>/exp.toml         【提交】项目契约
<仓库>/.exp/            每个检出各自的缓存，删掉不心疼
```

记录不在任何分支上，也不随 `clone` 走。备份与恢复：

```sh
exp backup     # 把记录提交成 book 分支
exp restore    # 在新克隆里恢复
```

## agent 契约

所有命令支持 `--json`（标准输出不是终端时自动切换），统一信封：

```json
{"ok": true, "data": {...}}
{"ok": false, "error": {"code": "conflict", "message": "..."}}
```

退出码：`0` 成功 · `1` 通用错误 · `2` 用法错误 · `3` 不存在 · `4` 校验失败 · `5` 冲突。
所有写操作支持 `--dry-run`；任何命令都不读标准输入。

## 测试

```sh
go test ./...
```

端到端用例会构建真实二进制、建临时仓库、真的起进程、真的杀进程。

## 文档

- `docs/INTENT.md` 为什么做
- `docs/CONTEXT.md` 术语
- `docs/DESIGN.md` 架构
- `docs/SPEC.md` 规格与验收
- `.scratch/expdeck/issues/` 实现票
