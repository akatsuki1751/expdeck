package gitx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// bookRef 是保存实验记录快照的 orphan 分支。
const bookRef = "refs/heads/book"

// gitEnv 在仓库根执行一条 git 命令，并附加环境变量。
func (r *Repo) gitEnv(env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Root
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

// tempIndex 建一个临时索引路径（文件本身先删掉，git 只接受不存在的索引）。
func tempIndex() (string, func(), error) {
	f, err := os.CreateTemp("", "expdeck-index-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp index: %v", err)
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", nil, fmt.Errorf("close temp index: %v", err)
	}
	if err := os.Remove(name); err != nil {
		return "", nil, fmt.Errorf("remove temp index: %v", err)
	}
	return name, func() { os.Remove(name) }, nil
}

// CommitTree 把 recordsDir 的内容提交成 book 分支上的一个快照。
//
// 记录目录位于 .git 下，普通 git add 会把它当成仓库内部路径而静默跳过，
// 所以这里把记录目录临时当成一个独立的工作区根：索引里只出现相对记录
// 目录的路径，还原时才能原样落回 recordsDir。
func (r *Repo) CommitTree(recordsDir, message string) (string, error) {
	dir, err := filepath.Abs(recordsDir)
	if err != nil {
		return "", fmt.Errorf("resolve records dir: %v", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("stat records dir: %v", err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("records path %s is not a directory", dir)
	}
	index, cleanup, err := tempIndex()
	if err != nil {
		return "", err
	}
	defer cleanup()

	stageEnv := []string{
		"GIT_INDEX_FILE=" + index,
		"GIT_DIR=" + r.CommonDir,
		"GIT_WORK_TREE=" + dir,
	}
	if _, err := r.gitEnv(stageEnv, "-C", dir, "add", "-A", "."); err != nil {
		return "", fmt.Errorf("stage records: %v", err)
	}
	out, err := r.gitEnv([]string{"GIT_INDEX_FILE=" + index}, "write-tree")
	if err != nil {
		return "", fmt.Errorf("write tree: %v", err)
	}
	tree := strings.TrimSpace(out)
	if tree == "" {
		return "", fmt.Errorf("write tree: empty output")
	}

	for attempt := 0; attempt < 3; attempt++ {
		parent := ""
		if r.BookExists() {
			out, err := r.gitEnv(nil, "rev-parse", "--verify", bookRef)
			if err != nil {
				return "", fmt.Errorf("read book ref: %v", err)
			}
			parent = strings.TrimSpace(out)
		}
		commit, err := r.commitTree(tree, parent, message)
		if err != nil {
			return "", err
		}
		args := []string{"update-ref", bookRef, commit}
		if parent != "" {
			args = append(args, parent)
		}
		if _, err := r.gitEnv(nil, args...); err != nil {
			if parent != "" {
				// 有别的进程抢先更新了 book：重读父提交再试。
				continue
			}
			return "", fmt.Errorf("update book ref: %v", err)
		}
		return commit, nil
	}
	return "", fmt.Errorf("update book ref: too many concurrent updates")
}

// commitTree 在给定 tree 上建提交；未配置 git 身份时用兜底身份重试一次。
func (r *Repo) commitTree(tree, parent, message string) (string, error) {
	args := []string{"commit-tree", tree}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	args = append(args, "-m", message)
	out, err := r.gitEnv(nil, args...)
	if err == nil {
		return strings.TrimSpace(out), nil
	}
	firstErr := err
	identity := []string{
		"GIT_AUTHOR_NAME=exp",
		"GIT_AUTHOR_EMAIL=exp@expdeck.local",
		"GIT_COMMITTER_NAME=exp",
		"GIT_COMMITTER_EMAIL=exp@expdeck.local",
	}
	out, err = r.gitEnv(identity, args...)
	if err != nil {
		return "", firstErr
	}
	return strings.TrimSpace(out), nil
}

// RestoreTree 把 book 分支的内容还原到 recordsDir。
func (r *Repo) RestoreTree(recordsDir string) error {
	if !r.BookExists() {
		return fmt.Errorf("%s does not exist", bookRef)
	}
	dir, err := filepath.Abs(recordsDir)
	if err != nil {
		return fmt.Errorf("resolve records dir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create records dir: %v", err)
	}
	index, cleanup, err := tempIndex()
	if err != nil {
		return err
	}
	defer cleanup()

	env := []string{"GIT_INDEX_FILE=" + index}
	if _, err := r.gitEnv(env, "read-tree", bookRef); err != nil {
		return fmt.Errorf("read book tree: %v", err)
	}
	prefix := filepath.ToSlash(dir)
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if _, err := r.gitEnv(env, "checkout-index", "-a", "-f", "--prefix="+prefix); err != nil {
		return fmt.Errorf("write records: %v", err)
	}
	return nil
}

// BookExists 报告备份分支是否存在。
func (r *Repo) BookExists() bool { return r.BranchExists("book") }

// BookFiles 返回 book 分支里的全部文件路径（相对记录目录）。
func (r *Repo) BookFiles() ([]string, error) {
	out, err := r.Git("ls-tree", "-r", "--name-only", "book")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}
