// note.go 实现实验笔记的读写：note.md 是实验的自由文本记录。

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"expdeck/internal/records"
)

const noteFileName = "note.md"

// NotePath 返回实验笔记文件的路径。
func (k *Kernel) NotePath(exp *Experiment) string {
	return filepath.Join(k.Store.ExpDir(exp.ID), noteFileName)
}

// ReadNote 读出笔记内容与版本；文件还不存在时视为空内容、版本为空。
func (k *Kernel) ReadNote(exp *Experiment) (string, string, error) {
	data, err := os.ReadFile(k.NotePath(exp))
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", nil
		}
		return "", "", err
	}
	content := string(data)
	return content, NoteVersion(content), nil
}

// WriteNote 在锁内整篇替换笔记。expectVersion 非空时要求与当前版本一致，
// 否则返回 ErrConflict。返回新内容的版本。
func (k *Kernel) WriteNote(exp *Experiment, content, expectVersion string) (string, error) {
	lock, err := k.Store.AcquireLock()
	if err != nil {
		return "", err
	}
	defer lock.Unlock()
	current, err := k.readNoteFile(exp)
	if err != nil {
		return "", err
	}
	if expectVersion != "" && expectVersion != NoteVersion(current) {
		return "", fmt.Errorf("%w: 笔记已被并发修改", ErrConflict)
	}
	if err := records.WriteFile(k.NotePath(exp), []byte(content)); err != nil {
		return "", err
	}
	return NoteVersion(content), nil
}

// AppendNote 在锁内读、追加一段带时间戳的小节、原子写，返回新版本。
func (k *Kernel) AppendNote(exp *Experiment, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%w: 追加内容不能为空", ErrInvalid)
	}
	lock, err := k.Store.AcquireLock()
	if err != nil {
		return "", err
	}
	defer lock.Unlock()
	current, err := k.readNoteFile(exp)
	if err != nil {
		return "", err
	}
	stamp := time.Now().Format(time.RFC3339)
	next := current + "\n## " + stamp + "\n" + text + "\n"
	if err := records.WriteFile(k.NotePath(exp), []byte(next)); err != nil {
		return "", err
	}
	return NoteVersion(next), nil
}

// NoteVersion 返回内容的 sha256 版本号（hex 全长）。
func NoteVersion(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// readNoteFile 读原始内容；文件不存在时返回空字符串。
func (k *Kernel) readNoteFile(exp *Experiment) (string, error) {
	data, err := os.ReadFile(k.NotePath(exp))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}
