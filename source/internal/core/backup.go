package core

import (
	"fmt"
	"os"
	"path/filepath"
)

// Backup 把记录目录提交到 book 分支，返回提交与文件数。
func (k *Kernel) Backup() (string, int, error) {
	lock, err := k.Store.AcquireLock()
	if err != nil {
		return "", 0, fmt.Errorf("%w: 获取记录锁失败: %v", ErrConflict, err)
	}
	defer lock.Unlock()

	commit, err := k.Repo.CommitTree(k.Store.Records(), "exp: backup")
	if err != nil {
		return "", 0, fmt.Errorf("%w: 备份记录失败: %v", ErrConflict, err)
	}
	booked, err := k.Repo.BookFiles()
	if err != nil {
		return commit, 0, fmt.Errorf("%w: 读取备份清单失败: %v", ErrConflict, err)
	}
	return commit, len(booked), nil
}

// Restore 把 book 分支的内容还原到记录目录。
func (k *Kernel) Restore(force bool) (int, error) {
	recordsDir := k.Store.Records()
	if !k.Repo.BookExists() {
		return 0, fmt.Errorf("%w: 备份分支 book 不存在，请先运行 exp backup", ErrNotFound)
	}
	if !force {
		nonEmpty, err := dirHasEntries(recordsDir)
		if err != nil {
			return 0, err
		}
		if nonEmpty {
			return 0, fmt.Errorf("%w: 记录目录 %s 非空（用 --force 覆盖）", ErrConflict, recordsDir)
		}
	}
	if err := k.Repo.RestoreTree(recordsDir); err != nil {
		return 0, fmt.Errorf("%w: 恢复记录失败: %v", ErrConflict, err)
	}
	booked, err := k.Repo.BookFiles()
	if err != nil {
		return 0, fmt.Errorf("%w: 读取备份清单失败: %v", ErrConflict, err)
	}
	return len(booked), nil
}

// UnbackedCount 返回磁盘上有、但 book 里没有对应路径的记录文件数。
//
// 这是近似值：只比较路径集合，不比较内容——逐文件比对内容太贵。
func (k *Kernel) UnbackedCount() (int, error) {
	disk, err := diskFiles(k.Store.Records())
	if err != nil {
		return 0, err
	}
	if !k.Repo.BookExists() {
		return len(disk), nil
	}
	booked, err := k.Repo.BookFiles()
	if err != nil {
		return 0, err
	}
	known := make(map[string]struct{}, len(booked))
	for _, path := range booked {
		known[filepath.ToSlash(path)] = struct{}{}
	}
	count := 0
	for _, path := range disk {
		if _, ok := known[path]; !ok {
			count++
		}
	}
	return count, nil
}

// dirHasEntries 报告目录是否存在且至少有一个条目。
func dirHasEntries(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return len(entries) > 0, nil
}

// diskFiles 列出目录下全部文件的相对路径（斜杠分隔）。
func diskFiles(dir string) ([]string, error) {
	fi, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%s 不是目录", dir)
	}
	var files []string
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
