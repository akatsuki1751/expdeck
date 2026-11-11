package core

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"expdeck/internal/records"
)

// Artifact 是一次运生产出的文件，按内容哈希入库。
type Artifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Store  string `json:"store"`
}

// fileStamp 描述一个文件在某个时刻的样子。
type fileStamp struct {
	Mod  int64 `json:"mod"`
	Size int64 `json:"size"`
}

// ObjectsDir 是产物库的根。
func (k *Kernel) ObjectsDir() string {
	return filepath.Join(k.Store.Root, "objects")
}

// manifestPath 是跑前清单的位置。
func (k *Kernel) manifestPath(exp *Experiment, runID string) string {
	return filepath.Join(k.RunDir(exp, runID), "manifest.json")
}

// ScanArtifacts 按通配扫出工作树里的候选文件。
func ScanArtifacts(worktree string, globs []string) (map[string]fileStamp, error) {
	out := map[string]fileStamp{}
	for _, pattern := range globs {
		matches, err := matchGlob(worktree, pattern)
		if err != nil {
			return nil, err
		}
		for _, abs := range matches {
			fi, err := os.Stat(abs)
			if err != nil || fi.IsDir() {
				continue
			}
			rel, err := filepath.Rel(worktree, abs)
			if err != nil {
				continue
			}
			out[filepath.ToSlash(rel)] = fileStamp{Mod: fi.ModTime().UnixNano(), Size: fi.Size()}
		}
	}
	return out, nil
}

// matchGlob 支持 `**`（filepath.Glob 不支持）。
func matchGlob(worktree, pattern string) ([]string, error) {
	pattern = filepath.ToSlash(pattern)
	if !strings.Contains(pattern, "**") {
		return filepath.Glob(filepath.Join(worktree, filepath.FromSlash(pattern)))
	}
	base := "."
	if idx := strings.Index(pattern, "**"); idx > 0 {
		base = strings.TrimSuffix(pattern[:idx], "/")
	}
	root := filepath.Join(worktree, filepath.FromSlash(base))
	re := regexp.MustCompile(globToRegexp(pattern))
	var out []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(worktree, path)
		if err != nil {
			return nil
		}
		if re.MatchString(filepath.ToSlash(rel)) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func globToRegexp(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '.':
			b.WriteString(`\.`)
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteString("$")
	return b.String()
}

// WriteManifest 在跑之前记下候选文件的现状。
func (k *Kernel) WriteManifest(exp *Experiment, runID, worktree string) error {
	config, err := k.LoadConfig()
	if err != nil {
		return err
	}
	manifest, err := ScanArtifacts(worktree, config.Artifacts)
	if err != nil {
		return err
	}
	return records.WriteJSON(k.manifestPath(exp, runID), manifest)
}

// CollectArtifacts 只收本次新增或修改过的文件，按内容哈希入库。
func (k *Kernel) CollectArtifacts(exp *Experiment, run *Run) error {
	config, err := k.LoadConfig()
	if err != nil {
		return err
	}
	if len(config.Artifacts) == 0 {
		return nil
	}
	var before map[string]fileStamp
	_ = records.ReadJSON(k.manifestPath(exp, run.ID), &before)
	if before == nil {
		before = map[string]fileStamp{}
	}
	after, err := ScanArtifacts(run.Cwd, config.Artifacts)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(after))
	for name := range after {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		stamp := after[name]
		if old, ok := before[name]; ok && old == stamp {
			continue // 跑之前就有、跑完没变：不是这次的产物
		}
		artifact, err := k.storeFile(run.Cwd, name)
		if err != nil {
			return err
		}
		run.Artifacts = append(run.Artifacts, artifact)
	}
	return nil
}

// storeFile 把一个文件按 sha256 放进产物库（同盘用硬链，避免复制）。
func (k *Kernel) storeFile(worktree, name string) (Artifact, error) {
	src := filepath.Join(worktree, filepath.FromSlash(name))
	digest, size, err := fileDigest(src)
	if err != nil {
		return Artifact{}, err
	}
	rel := filepath.Join("objects", digest[:2], digest)
	dst := filepath.Join(k.Store.Root, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return Artifact{}, err
	}
	if _, statErr := os.Stat(dst); statErr != nil {
		if linkErr := os.Link(src, dst); linkErr != nil {
			if err := copyFile(src, dst); err != nil {
				return Artifact{}, err
			}
		}
	}
	return Artifact{Name: name, SHA256: digest, Size: size, Store: filepath.ToSlash(rel)}, nil
}

func fileDigest(path string) (string, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	sum := sha256Hex(data)
	return sum, int64(len(data)), nil
}

// GC 删掉没有任何 run 引用的产物，返回删除数量。
func (k *Kernel) GC(dryRun bool) (int, error) {
	referenced := map[string]struct{}{}
	exps, err := k.List()
	if err != nil {
		return 0, err
	}
	for _, exp := range exps {
		runs, err := k.ListRuns(exp)
		if err != nil {
			return 0, err
		}
		for _, run := range runs {
			for _, artifact := range run.Artifacts {
				referenced[artifact.SHA256] = struct{}{}
			}
		}
	}
	removed := 0
	root := k.ObjectsDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	for _, bucket := range entries {
		if !bucket.IsDir() {
			continue
		}
		dir := filepath.Join(root, bucket.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			return removed, err
		}
		for _, file := range files {
			if _, ok := referenced[file.Name()]; ok {
				continue
			}
			if !dryRun {
				if err := os.Remove(filepath.Join(dir, file.Name())); err != nil {
					return removed, err
				}
			}
			removed++
		}
	}
	return removed, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
