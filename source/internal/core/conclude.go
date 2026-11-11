// conclude.go 实现结案与重开：结案把理由写进 conclusion.json，重开撤销它。

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"expdeck/internal/records"
)

// Conclusion 是一次结案留下的记录，与 meta.json 分开放。
type Conclusion struct {
	Decision string `json:"decision"`
	Why      string `json:"why"`
	At       string `json:"at"`
}

// Conclude 结案：keep 记为已得出结论，abandon 记为放弃。
func (k *Kernel) Conclude(exp *Experiment, decision, why string) error {
	switch decision {
	case "keep", "abandon":
	default:
		return fmt.Errorf("%w: decision 只接受 keep 或 abandon", ErrInvalid)
	}
	why = strings.TrimSpace(why)
	if why == "" {
		return fmt.Errorf("%w: 结案理由不能为空（--why）", ErrInvalid)
	}
	if exp.Status != StatusActive {
		return fmt.Errorf("%w: experiment already closed", ErrConflict)
	}
	if decision == "keep" {
		exp.Status = StatusConcluded
	} else {
		exp.Status = StatusAbandoned
	}
	rec := Conclusion{
		Decision: decision,
		Why:      why,
		At:       time.Now().Format(time.RFC3339),
	}
	if err := records.WriteJSON(k.conclusionPath(exp), rec); err != nil {
		return err
	}
	return records.WriteJSON(k.metaPath(exp), exp)
}

// Reopen 把已结案的实验改回 active，并删除结案记录。
func (k *Kernel) Reopen(exp *Experiment) error {
	if exp.Status == StatusActive {
		return fmt.Errorf("%w: 实验本来就处于 active 状态", ErrConflict)
	}
	prev := exp.Status
	exp.Status = StatusActive
	if err := records.WriteJSON(k.metaPath(exp), exp); err != nil {
		exp.Status = prev
		return err
	}
	if err := os.Remove(k.conclusionPath(exp)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// metaPath 返回一个实验的 meta.json 路径。
func (k *Kernel) metaPath(exp *Experiment) string {
	return filepath.Join(k.Store.ExpDir(exp.ID), "meta.json")
}

// conclusionPath 返回一个实验的 conclusion.json 路径。
func (k *Kernel) conclusionPath(exp *Experiment) string {
	return filepath.Join(k.Store.ExpDir(exp.ID), "conclusion.json")
}
