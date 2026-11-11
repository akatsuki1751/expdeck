// cmd_serve.go 提供 serve 命令：在本机启动 ExpDeck 网页。

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"expdeck/internal/core"
	"expdeck/internal/web"
)

// servePayload 是 serve 抢先输出的 JSON 信封，字段顺序固定为 ok、data。
type servePayload struct {
	OK   bool `json:"ok"`
	Data struct {
		Addr string `json:"addr"`
	} `json:"data"`
}

// cmdServe 启动网页服务并阻塞，直到监听失败或进程退出。
func (a *app) cmdServe(args []string) (result, error) {
	const usage = "usage: exp serve [--addr 127.0.0.1:8787]"
	addr := "127.0.0.1:8787"
	pos, err := splitArgs(args, map[string]*string{"addr": &addr}, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) > 0 {
		return result{}, fail(ExitUsage, "%s", usage)
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	if a.dry {
		return result{
			data: map[string]any{"dry_run": true, "addr": addr},
			human: func(w io.Writer) {
				fmt.Fprintf(w, "would serve on http://%s\n", addr)
			},
		}, nil
	}
	// 先自己监听，才能把 --addr 带 :0 时的实际端口写出去。
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return result{}, fail(ExitError, "监听 %s 失败: %v", addr, err)
	}
	actual := listener.Addr().String()
	if a.json {
		payload := servePayload{OK: true}
		payload.Data.Addr = actual
		if err := json.NewEncoder(a.env.Stdout).Encode(payload); err != nil {
			listener.Close()
			return result{}, fail(ExitError, "写出服务地址失败: %v", err)
		}
	} else {
		fmt.Fprintf(a.env.Stdout, "serving on http://%s\n", actual)
	}
	if err := http.Serve(listener, web.New(kernel).Handler()); err != nil {
		return result{}, fail(ExitError, "网页服务退出: %v", err)
	}
	return result{}, nil
}
