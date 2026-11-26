// Package core 是领域逻辑：CLI 与 Web 都只调这一层。
package core

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"expdeck/internal/gitx"
	"expdeck/internal/records"
)

// 实验状态。
const (
	StatusActive    = "active"
	StatusConcluded = "concluded"
	StatusAbandoned = "abandoned"
)

// 分类错误，供 CLI 映射退出码。
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid")
)

// Experiment 是实验的记录部分；它的代码身份是 Branch。
type Experiment struct {
	ID                string   `json:"id"`
	Path              []string `json:"path"`
	Branch            string   `json:"branch"`
	StartCommit       string   `json:"start_commit,omitempty"`
	Hypothesis        string   `json:"hypothesis"`
	Status            string   `json:"status"`
	RepresentativeRun string   `json:"representative_run,omitempty"`
}

// PathString 返回斜杠连接的可读路径。
func (e *Experiment) PathString() string { return strings.Join(e.Path, "/") }

// Kernel 组合仓库与记录库。
type Kernel struct {
	Repo  *gitx.Repo
	Store records.Store
}

// Open 在 dir 里打开一个已接入的项目。
func Open(dir string) (*Kernel, error) {
	repo, err := gitx.Open(dir)
	if err != nil {
		return nil, err
	}
	return &Kernel{Repo: repo, Store: records.New(repo.CommonDir)}, nil
}

// Init 建立数据根骨架。
func (k *Kernel) Init() error { return k.Store.InitLayout() }

// List 读出全部实验，按路径排序。
func (k *Kernel) List() ([]*Experiment, error) {
	entries, err := os.ReadDir(k.Store.ExpsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Experiment
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var exp Experiment
		if err := records.ReadJSON(filepath.Join(k.Store.ExpDir(entry.Name()), "meta.json"), &exp); err != nil {
			continue
		}
		out = append(out, &exp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PathString() < out[j].PathString() })
	return out, nil
}

// Find 按路径或标识找实验。
func (k *Kernel) Find(ref string) (*Experiment, error) {
	want := NormalizePath(ref)
	exps, err := k.List()
	if err != nil {
		return nil, err
	}
	for _, exp := range exps {
		if exp.ID == ref || exp.PathString() == want {
			return exp, nil
		}
	}
	return nil, fmt.Errorf("%w: experiment %q", ErrNotFound, ref)
}

// Create 建分支并落记录。为什么 必须给。
func (k *Kernel) Create(pathStr, why string) (*Experiment, error) {
	if strings.TrimSpace(why) == "" {
		return nil, fmt.Errorf("%w: hypothesis is required (--why)", ErrInvalid)
	}
	segs, err := ParsePath(pathStr)
	if err != nil {
		return nil, err
	}
	exps, err := k.List()
	if err != nil {
		return nil, err
	}
	for _, exp := range exps {
		if isPrefix(exp.Path, segs) || isPrefix(segs, exp.Path) {
			return nil, fmt.Errorf("%w: %q overlaps with existing experiment %q", ErrConflict, pathStr, exp.PathString())
		}
	}
	branch := "exp/" + strings.Join(segs, "/")
	if k.Repo.BranchExists(branch) {
		return nil, fmt.Errorf("%w: branch %q already exists", ErrConflict, branch)
	}
	start, err := k.Repo.Git("rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return nil, fmt.Errorf("%w: 无法确定实验起点: %v", ErrConflict, err)
	}
	if err := k.Repo.CreateBranch(branch); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConflict, err)
	}
	exp := &Experiment{
		ID:          NewID(),
		Path:        segs,
		Branch:      branch,
		StartCommit: strings.TrimSpace(start),
		Hypothesis:  strings.TrimSpace(why),
		Status:      StatusActive,
	}
	if err := records.WriteJSON(filepath.Join(k.Store.ExpDir(exp.ID), "meta.json"), exp); err != nil {
		return nil, err
	}
	return exp, nil
}

// NormalizePath 去掉可选的 exp/ 前缀、首尾斜杠与空白。
func NormalizePath(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "exp/")
	s = strings.Trim(s, "/")
	return s
}

// ParsePath 把 "a/b/c" 拆成合法分段。
func ParsePath(s string) ([]string, error) {
	norm := NormalizePath(s)
	if norm == "" {
		return nil, fmt.Errorf("%w: experiment path is required", ErrInvalid)
	}
	segs := strings.Split(norm, "/")
	for _, seg := range segs {
		switch {
		case seg == "", seg == ".", seg == "..":
			return nil, fmt.Errorf("%w: invalid path segment %q", ErrInvalid, seg)
		case strings.ContainsAny(seg, " ~^:?*[\\"):
			return nil, fmt.Errorf("%w: path segment %q contains an illegal character", ErrInvalid, seg)
		}
	}
	return segs, nil
}

func isPrefix(a, b []string) bool {
	if len(a) > len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Node 是展示用的树节点。
type Node struct {
	Name     string
	Path     []string
	Exp      *Experiment
	Children []*Node
}

// BuildTree 把平铺的实验折成嵌套树。
func BuildTree(exps []*Experiment) *Node {
	root := &Node{Name: "."}
	index := map[string]*Node{"": root}
	for _, exp := range exps {
		parent := root
		for i := range exp.Path {
			key := strings.Join(exp.Path[:i+1], "/")
			node, ok := index[key]
			if !ok {
				node = &Node{Name: exp.Path[i], Path: exp.Path[:i+1]}
				index[key] = node
				parent.Children = append(parent.Children, node)
			}
			parent = node
		}
		parent.Exp = exp
	}
	sortNodes(root)
	return root
}

func sortNodes(n *Node) {
	sort.Slice(n.Children, func(i, j int) bool { return n.Children[i].Name < n.Children[j].Name })
	for _, child := range n.Children {
		sortNodes(child)
	}
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewID 生成一个 ULID：时间戳可排序，后 80 位随机。
func NewID() string {
	var raw [16]byte
	ms := uint64(time.Now().UnixMilli())
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	if _, err := rand.Read(raw[6:]); err != nil {
		for i := 6; i < 16; i++ {
			raw[i] = byte(time.Now().UnixNano() >> (uint(i) % 8))
		}
	}
	var out [26]byte
	for i := 0; i < 26; i++ {
		v := 0
		for j := 0; j < 5; j++ {
			bit := i*5 + j - 2
			v <<= 1
			if bit >= 0 && bit < 128 && (raw[bit/8]>>uint(7-bit%8))&1 == 1 {
				v |= 1
			}
		}
		out[i] = crockford[v]
	}
	return string(out[:])
}
