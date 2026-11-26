package gitx

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
)

// RenameBookExperiment updates only the saved identity in an existing book snapshot.
// A branch without this experiment is left untouched.
func (r *Repo) RenameBookExperiment(id string, segments []string, branch string) error {
	if !r.BookExists() {
		return nil
	}
	name := path.Join("exps", id, "meta.json")
	parentText, err := r.Git("rev-parse", "--verify", bookRef)
	if err != nil {
		return err
	}
	parent := strings.TrimSpace(parentText)
	if _, err = r.Git("cat-file", "-e", parent+":"+name); err != nil {
		return nil
	}
	raw, err := r.Git("show", parent+":"+name)
	if err != nil {
		return err
	}
	var meta map[string]json.RawMessage
	if err = json.Unmarshal([]byte(raw), &meta); err != nil {
		return fmt.Errorf("解析 book 记录失败: %w", err)
	}
	meta["path"], err = json.Marshal(segments)
	if err != nil {
		return err
	}
	meta["branch"], err = json.Marshal(branch)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.CreateTemp("", "expdeck-book-meta-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	blob, err := r.Git("hash-object", "-w", file.Name())
	if err != nil {
		return err
	}
	index, cleanup, err := tempIndex()
	if err != nil {
		return err
	}
	defer cleanup()
	env := []string{"GIT_INDEX_FILE=" + index}
	if _, err = r.gitEnv(env, "read-tree", parent); err != nil {
		return err
	}
	if _, err = r.gitEnv(env, "update-index", "--add", "--cacheinfo", "100644,"+strings.TrimSpace(blob)+","+name); err != nil {
		return err
	}
	tree, err := r.gitEnv(env, "write-tree")
	if err != nil {
		return err
	}
	commit, err := r.commitTree(strings.TrimSpace(tree), parent, "exp: rename experiment")
	if err != nil {
		return err
	}
	if _, err = r.Git("update-ref", bookRef, commit, parent); err != nil {
		return fmt.Errorf("更新 book 失败: %w", err)
	}
	return nil
}
