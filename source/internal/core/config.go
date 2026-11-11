package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProjectConfig 是 exp.toml 里本工具用得到的部分。
// 只解析本工具生成的 TOML 子集：section + `key = "value"`。
type ProjectConfig struct {
	RunCmd    string
	RunCwd    string
	Timeout   time.Duration
	Store     string
	Artifacts []string
	Keep      string
}

// LoadConfig 读取仓库根的 exp.toml。
func (k *Kernel) LoadConfig() (*ProjectConfig, error) {
	path := filepath.Join(k.Repo.Root, "exp.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: 缺少 exp.toml，请先运行 exp init", ErrInvalid)
		}
		return nil, err
	}
	cfg := &ProjectConfig{RunCwd: "."}
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if idx := strings.Index(value, "#"); idx >= 0 && !strings.HasPrefix(value, `"`) {
			value = strings.TrimSpace(value[:idx])
		}
		value = strings.Trim(value, `"`)
		switch section {
		case "run":
			switch key {
			case "cmd":
				cfg.RunCmd = value
			case "cwd":
				cfg.RunCwd = value
			case "timeout":
				if duration, parseErr := time.ParseDuration(value); parseErr == nil {
					cfg.Timeout = duration
				}
			}
		case "store":
			if key == "path" {
				cfg.Store = value
			}
		case "artifacts":
			switch key {
			case "glob":
				cfg.Artifacts = parseStringArray(value)
			case "keep":
				cfg.Keep = value
			}
		}
	}
	return cfg, nil
}

// parseStringArray 解析 `["a", "b"]` 这种最小 TOML 数组。
func parseStringArray(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, `"'`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
