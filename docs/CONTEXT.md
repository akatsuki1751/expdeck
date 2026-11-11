# ExpDeck

一套围绕 git 的实验记录与编排系统。人用浏览器审阅，agent 用命令行操作。这个文件的唯一作用是固定词汇，让讨论、代码、界面说同一个词。

## Language

### 实体

**ExpDeck**:
本工具。命令行 `exp`，项目配置 `exp.toml`，本地缓存 `.exp/`，数据根 `.git/exp/`。
_Avoid_: Expedition、xpd、expmgr

**Experiment（实验）**:
一条以 `exp/` 开头的分支，配一个假设。是**并行**的单位，内部 run 严格串行。
_Avoid_: 实验组、project、task

**Run（运行）**:
对某个快照的一次执行。携带提交、配置、环境、指标、产物。指标属于 run，不属于 experiment。
_Avoid_: job、task、trial

**Folder（文件夹）**:
实验树上的分组节点，只由路径前缀决定，本身不是实体，没有生命周期。
_Avoid_: 集合、分组、project

**Worktree（工作树）**:
一个实验被挂载后，磁盘上那份可编辑的目录。

### 生命周期

**attach（挂载）**:
把分支变成工作树，使它可编辑、可运行。可逆。

**detach（卸载）**:
把工作树收回成普通分支，目录消失、分支保留。可逆。
_Avoid_: 关闭、开启、结束

**conclude（结案）**:
给实验写上结论，表示这条线已有定论。不改变挂载状态。

**abandon（放弃）**:
结案的一种，表示不再继续，原因必须记录。
_Avoid_: 杀掉、删除

**reopen（重开）**:
把已结案的实验重新打开。

> 「关闭」是禁词：它同时被用来指 detach（可逆的物理动作）和 conclude（语义终局）。出现时必须追问指哪一个。

### 记录

**Records（记录）**:
`.git/exp/records/` 下的文本文件：实验、笔记、run。是记录的唯一真相；不进版本库，不随 clone 走。
_Avoid_: 台账、ledger、数据库

**book（备份）**:
一条 orphan 分支，存 Records 的历史快照。只由 `exp backup` / `exp restore` 读写，不检出、不参与合并。
_Avoid_: ledger 分支、xpd-ledger

**Snapshot（快照）**:
run 开始前对**代码工作区**打的游离提交；不进分支历史，只用来算 run 之间的增量。
_Avoid_: checkpoint（那是权重）

**Pointer（指针）**:
实验指向某个 run 的引用。**head** 指最近的 valid run；**best** 指按主指标最好的 run。

**Artifact（产物）**:
run 产出的文件，按内容哈希存在 `objects/`，`run.json` 里只留指针。

**Metric（指标）**:
从产物或评估里算出的数值，写回发起它的那个 run。

### 命令与视图

**cmd（命令）**:
`exp.toml` 里声明的一项可执行能力。带 `kind`（plot / table / eval）与 `scope`（exp / all）。web 右侧栏的可选项由它生成。

**Registry（登记表）**:
`.git/exp/registry/` 下「谁在跑、刚跑完什么」的短命缓存。不是真相，丢了可从 Records 重建。

**Cache（缓存）**:
`.exp/` 目录，每个 checkout 一份，只放渲染产物和终端历史。删掉不心疼。

### 状态

**valid**:
结果可采信、可作为结论依据的 run。

**crashed**:
进程异常消失（被杀、OOM、断电）的 run，没有正常退出码。

**interrupted**:
被人为中断的 run。留在记录里，但不算 valid。

**slop**:
agent 为改代码临时写的一次性脚本。挂在它服务的那个 run 上，而不是放在项目里。
