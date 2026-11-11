# DESIGN

架构与数据结构。术语见 `CONTEXT.md`，意图见 `INTENT.md`。
标 **暂定** 的是提案，尚未拍板。

---

## 架构：一个二进制，两个前端，一个内核

```
        ┌─────────────┐        ┌──────────────┐
        │  CLI 前端    │        │  Web 前端     │
        │ exp new/run │        │ exp serve    │
        │ show/wait…  │        │ 审阅 + 触发   │
        └──────┬──────┘        └──────┬───────┘
               └─────────┬────────────┘
                  ┌──────▼───────┐
                  │  internal/   │  内核：读记录、写记录、
                  │    core      │  算状态、算 diff
                  └──────┬───────┘
        ┌────────────────┼────────────────┐
   ┌────▼────┐     ┌─────▼─────┐    ┌─────▼─────┐
   │ records │     │  runner   │    │  objects  │
   │ (文件)   │     │  (进程)   │    │  (哈希)   │
   └─────────┘     └───────────┘    └───────────┘
```

**一个二进制，两个入口。** `exp run …` 和 `exp serve` 是同一个程序的两个子命令，前端代码分两个包，共用 `internal/core`。

**不做独立 kernel 进程。** 独立进程要付三笔账：IPC（协议、超时、序列化）、生命周期（谁拉起、挂了怎么办）、状态归属（内存和磁盘不一致算谁的）。做成一个 Go 包，三笔账全是零。

**不需要常驻守护。** 两个前端读的是同一份磁盘状态，不需要中间人。任何时刻所有进程都能杀掉，重启后靠解析恢复。

```
cmd/exp/          # 两个前端的入口
internal/core/    # 内核：Experiment/Run 的加载、写入、状态推导
internal/ledger/  # 记录读写（普通文件）+ book 分支的备份/恢复
internal/runner/  # 起进程、监督、写登记表
internal/store/   # 内容寻址的产物库
internal/web/     # 模板、路由、SSE
```

---

## 组件（进程视角）

| 进程 | 生命周期 | 职责 |
|---|---|---|
| `exp …` | 一次调用 | CLI 动作；agent 的主通道 |
| `exp serve` | 随手启停 | web；读记录、触发命令 |
| 监督进程 `exp __watch` | 每个 run 一个，随 run 生灭 | 起训练进程、等退出、写记录、发 hook |
| `records/` | 目录 | 记录本体：实验、笔记、run |
| `objects/` | 目录 | 产物：权重、npz，按内容哈希 |
| `book` 分支 | git ref | records 的历史快照，只用于备份/恢复 |

---

## 位置

**一个根，就在仓库里。** 没有 `~/.exp`。

```
<repo>/
├── exp.toml                      # 提交：项目契约
│
├── .git/
│   └── exp/                      # 真相；所有 worktree 共享；不进版本库
│       ├── records/              # 实验与 run（纯文本）
│       ├── objects/cc/08f6….pth  # 产物（权重、npz），按 sha256
│       ├── registry/<run_id>.json# 谁在跑、刚跑完什么；可重建
│       ├── gpus/3.lock           # GPU 占位
│       ├── boot                  # 上次的 boot_id，用于判断断电
│       ├── logs/<run_id>.log     # 训练日志；不进 records，也不进备份
│       └── cache/                # 可重建的中间数据
│
└── .exp/                         # 每个 checkout 一份；只有缓存；删了不心疼
    ├── context.json              # 我在哪个实验（脱离分支时兜底）
    ├── out/<hash>                # 渲染产物
    └── terminals/<exp>/history   # 这个实验的 shell 历史
```

**为什么记录在 `.git/` 里而不是和 `.git` 平级：**

1. `.git` 是**唯一被所有 worktree 共享**的目录。`<repo>/.exp` 只有主检出有，而 `exp new` 建的 worktree 是别的目录。
2. `git clean -xdf` 会删掉 gitignore 的 `.exp`，但碰不到 `.git/`。
3. 路径用 `git rev-parse --git-common-dir` 拿，在任何 worktree、`--separate-git-dir`、bare 仓库下都对。

它不是版本库的一部分：`git status` 看不见，切分支不带走，`git gc` 不碰它。git-lfs 同样把对象存在 `.git/lfs/`。

**`.exp/` 的判据：删掉不心疼。** 有价值的东西在 `records/` 和 `objects/`，一次性的挂 run，缓存才放 `.exp/`。

---

## 实体模型

**两个 entity 加一条父子边**，不是更多。

```go
type Experiment struct {
    ID         string     // ULID
    Path       []string   // ["sensitive", "hyperparameter"]
    Branch     string
    Hypothesis string
    Note       string     // markdown，实验日志；人可写、agent 可追加
    Status     string     // active | concluded | abandoned
    Runs       []*Run     // 串行；按 run_id 排序（字典序即时间序）
}

type Run struct {
    ID        string
    Snapshot  string      // 游离提交：权重 → 代码的全部答案在这里
    Base      string      // 上一个快照
    Status    string      // running | finished | interrupted | crashed
    Outcome   string      // valid | invalid | ""
    Metrics   map[string]float64
    Artifacts []Artifact
}

// 内核持有的全部状态
type Kernel struct {
    exps map[string]*Experiment
}
```

Run 不是独立实体——脱离实验它没有身份，像 commit 脱离仓库。加载 = 把 `records/` 读一遍塞进 map，三十个实验几毫秒。

### 选型判据

> **选数据结构之前，先问「我最大的集合有多大」。**

| 集合规模 | 用什么 |
|---|---|
| 两位数（本项目的现状） | map / 切片 / 现排 |
| 六位数以上 | 才谈树、索引文件、B+ |

**不手写平衡树。** 红黑树的意义是「海量键 + 有序 + O(log n)」，这里 n 是两位数；Go 的 map 是 O(1) 且零行代码，而手写平衡树等于**引入一个只有一个贡献者、没有测试的库**。需要的有序各自二十行就够：

- run 按时间：`run_id` 自带时间序
- 路径树：path 排一次序，扫一遍折叠成嵌套结构
- 范围查询：有序切片 + `sort.Search`

---

## 记录与备份

- **真相是文件**：`.git/exp/records/` 里的普通文本。写记录就是写文件，没有 plumbing，没有 `update-ref`。
- **历史是 `book` 分支**：`exp backup` 把 `records/` 整棵树提交成 `book` 上的一个 commit；`exp restore` 从 `book` 把树落回来。
- `book` 是 orphan 分支，不检出、不参与合并，`git log main` 看不见它。
- 克隆之后 `records/` 是空的（`.git` 不随 `clone` 走），`exp restore` 一条命令重建。

类比：`records/` 是当下（工作区），`book` 是历史（提交）。区别只是这里的工作区在 `.git` 里面。

**为什么有了 `.git/exp/records/` 还要 `book`？** 因为 records 是文件，不是历史：`git clone` 不带走它，`rm -rf .git` 连根删掉，换台机器就什么都没有。`book` 是它的历史与备份——和「工作区文件 + git 提交」完全同构，只是这里的工作区在 `.git` 里面。

**备份是显式的，不是每次写入都提交。** 三十个实验的 records 也就几十 KB，随手 `exp backup` 的成本可以忽略；把提交塞进每次写入只会制造噪音，并引入"写一半失败"的中间态。

---

## 记录的布局

```
records/
└── exps/<exp_id>/
    ├── meta.json                # 路径、分支、假设、状态、结论
    ├── note.md                  # 实验日志；web 可编辑
    └── runs/<run_id>/
        ├── run.json
        └── scripts/             # slop：为这次 run 写的一次性脚本
```

`exp_id` 是 ULID，`exps/` 下是**平铺**的；三层文件夹从 `meta.json.path` 算出来，不是目录结构。run 嵌在实验里面，**删一个实验就是删一个目录**——所有 run 一起走，不会留下孤儿。

训练日志不在 `records/` 里（否则备份体积由日志决定），它在 `.git/exp/logs/<run_id>.log`，`run.json` 只存路径。

```json
// exps/01JABCDEF…/meta.json
{
  "id": "01JABCDEF…",
  "path": ["sensitive", "hyperparameter"],
  "branch": "exp/sensitive/hyperparameter",
  "hypothesis": "lr 降到 1e-4 能提升 r2",
  "status": "active"
}
```

`head` / `best` **不存**：从 run 记录算出来。`eval` 补了指标之后 `best` 会变，存下来必然过期。

---

## 笔记

笔记属于**实验**，不属于 run。它是一条实验线的日志：假设写在开头，观察一条条追加，结论写在结案时。run 目录里没有笔记，run 只留事实（`run.json`）和它用过的一次性脚本。

```
exp note <exp>                 # 替换整篇
exp note <exp> --append "…"    # 追加一段，带时间戳
exp open <exp>                 # 调 $EDITOR 改整篇
```

**web 的编辑**：读态渲染 markdown，点「编辑」切纯文本 textarea，显式保存（Ctrl/Cmd+S）。不做实时预览、不做自动保存、不做语法检查、不做大纲——jupyter 也没有这些。

**唯一写入口**：`exp note`、`--append`、`open` 的保存、web 的保存，全部走内核里的同一个函数。它做一件事：

1. 拿独占文件锁
2. 读当前内容、算哈希
3. 和调用方给的期望哈希比对，不一致就返回冲突
4. 写临时文件 + rename
5. 放锁

前三步必须在同一把锁里。否则两个写入者会**同时通过校验**——两边都读到版本 A、都以为没变、都写，后写的覆盖先写的，而且两边都显示"保存成功"。这就是文件系统没有 git 那种 CAS 时必须自己补的东西。

---

## 索引与搜索

**不引入数据库。** 记录就是文件树，所有查询现读现算：

| 查询 | 做法 | 量级 |
|---|---|---|
| 全部实验 / run | 遍历 `records/` | 数十个文件，毫秒级 |
| 总表（按指标排序） | 读全部 `run.json` 的 `metrics` | 同上 |
| 搜笔记 | 扫 md/json 文本 | 线性，够用 |
| 谁在跑 | 读 `registry/` 目录 | 几个文件 |

git 本身也没有查询能力：它有一个按内容寻址的对象堆和一个 refs 列表，能回答「给我这个哈希」，不能回答「给我 r2 > 0.8 的 run」。这类问题在这个量级上，扫一遍就是答案。

规模假设：一个项目十几个到几十个实验，每个实验几次到几十次 run。这个量级上不需要索引或缓存；**但这条判断以实测延迟为准**——真慢到肉眼可见时，再加一个删了能自动重建的索引。

好处不只是简单：页面可以把整个项目状态一次性渲染出来，没有分页、没有懒加载；`exp recall` 也不必做相似度检索，直接把所有实验的一句话摘要倒出来——**检索会漏，列举不会。**

---

## 不做反查

论文写作时「这个数字属于哪个实验」已经由集中目录的分类法解决，那是稳定习惯，工具不该重复建模。

工具只负责一件事：**从权重能回到代码和配置**。这条链在快照里，不需要额外索引。

所以没有 `exp trace <file>`。入口只有正查：

- `exp show <run_id>` → 快照、diff、配置、命令、假设、结论
- `exp show --exp sensitive/hyperparameter` → 该实验全部 run

真需要反查那天，遍历所有 `run.json` 的 `artifacts` 就是答案（几十毫秒），不预设索引。

---

## `exp.toml`

```toml
[project]
name = "Patchflow-JEPA"

[tree]
prefix = "exp"                    # 暂定：一个仓库一棵树

[store]
path = ""                         # 空 = .git/exp/objects；可指向别的盘

[run]
cmd     = "bash run.sh --config {config}"
cwd     = "."
timeout = "12h"

[run.env]
PYTHONPATH = "."

[metrics]
file      = "logs/**/test_metrics.json"
primary   = "r2"
direction = "max"                 # best 指针据此排序；没有指标时退化为 head

[artifacts]
glob = ["logs/**/best_model.pth"]
keep = "best"                     # best | all | none

[[cmds]]
name  = "plot"
cmd   = "python tools/plot.py --exp {exp} --out {out}"
kind  = "plot"                    # plot | table | eval
scope = "exp"                     # exp: 右栏；all: 左栏总图

[hooks]                           # 暂定
on_run_start = ""                 # run 开始前执行，例如 pip freeze > .exp/env.txt
on_finish    = ""                 # 跑完执行，用于唤醒 agent 框架
```

工具**不理解参数**。`{config}` 只是字符串替换，实际执行的完整命令写进 `run.json`。

---

## `run.json`

```jsonc
{
  "schema": 1,
  "run_id": "20261111-142912-7f3a",
  "exp": "exp_id",
  "status": "running",            // running | finished | interrupted | crashed
  "outcome": null,                // valid | invalid | null（未评）
  "commit": "52ba741…",           // 快照对应的游离提交
  "base": "…",                    // 上一个 run 的快照，或实验起点
  "cmd": "bash run.sh --config configs/clean/pretrain.yaml",
  "cwd": "~/wt/attention",
  "env": { "CUDA_VISIBLE_DEVICES": "3", "EXP_RUN_ID": "…" },
  "started_at": "…", "finished_at": "…",
  "exit_code": 0,
  "pid": 203189, "boot_id": "…", "pgid": 203189,
  "metrics": { "r2": 0.8756, "mae": 0.1983 },
  "artifacts": [
    { "name": "stage1.best_model", "sha256": "cc08…", "size": 1, "store": "objects/cc/08f6…" }
  ],
  "log": { "path": "…/stdout.log", "tail_sha": "…" }
}
```

---

## 状态判定

**结局只写一处：`run.json`。** `registry/` 只回答「现在谁活着」，它可以从 `run.json` + 进程存在性重建。

判定顺序：

1. 读 `run.json.status`。不是 `running` 就是结局，直接信。
2. 是 `running` → 查进程，用 `pid + 进程启动时间` 确认还是同一个进程（防 pid 复用）。
   - 活着 → 仍在跑
   - 死了 → 结局是 `crashed`，补写 `run.json`
3. `boot_id` 变了（机器重启过）→ 所有 `running` 一律判死。

**监督进程只是记录员，不是真相。** 它正常结束时立刻写 `run.json`；它被 `kill -9`、或者它死时训练还活着（`setsid` 让训练脱离了它），都不会让系统误判：

- 训练还活着 → 仍算在跑，仍占据这个实验和那张卡，**拒绝第二次启动**
- 训练也死了 → 下次任何 `exp` 调用把 `run.json` 收敛成 `crashed`

旧设计让 `run.json` 和 `registry` 都写结局、查询又优先读 registry：两次写之间被杀，CLI 说完成、Web 说在跑、`exp wait` 永远等不到。所以结局只能有一个写入者。

两个前端都只是读这套数据，不持有状态。

---

## 快照

每个 run 开始前：

1. 用临时索引（`GIT_INDEX_FILE`）+ `git add -A` 把工作区全量（含未跟踪）写进 tree
2. `git commit-tree` 造一个提交，父提交是上一个 run 的快照
3. **把 `refs/exp/snap/<exp_id>` 指向它**
4. 记 `run.json.commit`

第 3 步不能省。**没有 ref 指向的提交，`git gc` 两周后就会清掉**——`run.json` 里那串哈希变成 `fatal: bad object`：指标在、笔记在、代码没了，整个产品的核心承诺当场失效。

因为每个快照的父提交是上一个快照，它们天然是一条链：**每个实验只需一个 ref 指向链尾，整条历史都活着**。

这些 ref 不会随 `clone` / `push` 自动走（默认只传分支和标签），所以 `exp backup` 要显式带上它们，`exp restore` 也要显式拉回来。

于是三种 diff 都是同一套数据的视图：

| 视图 | 范围 |
|---|---|
| 这个 run 改了什么 | 上个快照 → 本快照 |
| 这个实验改了什么 | 实验起点 → head |
| 跟另一个实验比 | 两个 exp 分支的 merge-base → 对方 |

注意：快照是**代码仓库**里的游离提交，和 `records/` 无关；`book` 里存的是记录，不是代码。

---

## 执行与中断

```
exp run ──► 起监督进程 ──► setsid 训练进程
                │                  │
                ├─ 写 run.json(running)、占 GPU 位
                ├─ 日志重定向到 records/runs/<id>/stdout.log
                ├─ 等待退出 ───────┘
                ├─ 写 run.json(exit_code)、撤销存活标记、释放 GPU 位、跑 hook
                └─ 退出
```

- 两个前端落一个 run 之后立刻返回，页面转圈靠读 registry + 日志尾
- 中断 = 给进程组发 `SIGINT`，10 秒后 `SIGTERM`，30 秒后 `SIGKILL`；标记 `interrupted`
- 关浏览器、关终端、SSH 断开都不影响（进程已 `setsid`）

---

## 产物归属

`[artifacts] glob` 是通配符，复用同一个 worktree 时会匹配到上一次留下的文件：第二次训练第 3 分钟就崩了，产物采集照样扫到上次的 `best_model.pth`（0.87），把它记到这次头上。打开总表，你以为"换参数没影响"。

规则：

1. run 开始前，把 glob 匹配到的文件记一份清单（路径 + 修改时间 + 大小）
2. 结束时**只接受新增或修改过**的文件
3. 脚本可以读 `EXP_OUT`（每次 run 的隔离输出目录），直接写那里——比靠通配符猜更可靠

指标走同一份清单，否则会读到上一次的 `test_metrics.json`。

---

## 并发规则

| 资源 | 规则 |
|---|---|
| 实验 | 同一实验同时只能有一个 run（串行语义），第二个直接拒 |
| GPU | `--gpu n` 写 `.git/exp/gpus/n.lock`；冲突则拒。另读 `nvidia-smi` 提示外部占用 |
| worktree | 路径带短 id，同名实验不撞 |
| 记录 | 单进程写 + 文件锁；`exp backup` 也是单进程 |

GPU 占位是**每个仓库各自一份**，跨仓库互相看不见——机器是共用的，真相以 `nvidia-smi` 为准，占位只防自己项目的两个 agent 撞车。暂定。

---

## 路径层级

`exp/sensitive/hyperparameter` → 三层文件夹。

- 分支名由 CLI 从路径派生；路径本身记在 `meta.json.path`
- **文件夹与实验是两种节点**：不允许在同一个位置既有文件夹又有实验（交互上挡住），git 的 D/F 冲突因此不会发生
- 改名用 `exp mv`：改 `meta.json`、`git branch -m`、搬 worktree，一条命令

---

## 验收（两条先跑通，再谈别的）

1. **代码能找回**：脏工作区跑一次 run → `exp backup` → 在新克隆里 `exp restore` → `exp show <run_id>` 仍能展示当时的代码和 diff。
2. **不会重复开跑**：`kill -9` 监督进程、训练仍在跑 → `exp run` 仍被拒 → CLI、Web、`exp wait` 看到同一个答案。

这两条跑不通，总表、图表、agent 唤醒都是空中楼阁。

---

## 暂定 / 未定

1. **项目边界**：一个仓库一棵树；`[tree] prefix` 预留
2. **attach 恢复**：默认不自动恢复脏改动，只提示；`--restore-dirty` 显式恢复
3. **唤醒**：`[hooks] on_finish` + `exp wait`，工具不绑定任何 agent 框架
4. **registry 清理**：`seen = true` 后保留 7 天再删
5. **环境快照**：不内建，也不做 commit hook。理由：commit 的时机会错（环境是 run 的属性，而且允许 dirty 跑）、会污染代码 diff、conda 环境里 `pip freeze` 不全、`-e` 路径会把机器路径带进仓库。需要时自己挂 `on_run_start`，写进 `.exp/`（gitignore，不污染 diff）。
6. **备份时机**：显式 `exp backup`；是否在 detach / 结案时自动触发一次，待定。
