package external

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/main/confloader"
)

func ConfigLoader(arg string) (out io.Reader, err error) {
	var data []byte
	switch {
	case strings.HasPrefix(arg, "http+unix://"):
		data, err = FetchUnixSocketHTTPContent(arg)

	// 从指定url读取数据，保存到字节切片
	case strings.HasPrefix(arg, "http://"), strings.HasPrefix(arg, "https://"):
		data, err = FetchHTTPContent(arg)

	// 从stdin读取数据，保存到字节切片
	case arg == "stdin:":
		data, err = io.ReadAll(os.Stdin)

	// 从文件读取数据，保存到字节切片
	default:
		data, err = os.ReadFile(arg)
	}

	if err != nil {
		return
	}
	// bytes.NewBuffer：从一个字节切片，构造一个buffer，Buffer类型有一个Read方法，实现了io.Reader接口
	out = bytes.NewBuffer(data)
	return
}

func FetchHTTPContent(target string) ([]byte, error) {
	parsedTarget, err := url.Parse(target)
	if err != nil {
		return nil, errors.New("invalid URL: ", target).Base(err)
	}

	if s := strings.ToLower(parsedTarget.Scheme); s != "http" && s != "https" {
		return nil, errors.New("invalid scheme: ", parsedTarget.Scheme)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(&http.Request{
		Method: "GET",
		URL:    parsedTarget,
		Close:  true,
	})
	if err != nil {
		return nil, errors.New("failed to dial to ", target).Base(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.New("unexpected HTTP status code: ", resp.StatusCode)
	}

	content, err := buf.ReadAllToBytes(resp.Body)
	if err != nil {
		return nil, errors.New("failed to read HTTP response").Base(err)
	}

	return content, nil
}

// Format: http+unix:///path/to/socket.sock/api/endpoint
func FetchUnixSocketHTTPContent(target string) ([]byte, error) {
	path := strings.TrimPrefix(target, "http+unix://")

	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("unix socket path must be absolute")
	}

	var socketPath, httpPath string

	sockIdx := strings.Index(path, ".sock")
	if sockIdx != -1 {
		socketPath = path[:sockIdx+5]
		httpPath = path[sockIdx+5:]
		if httpPath == "" {
			httpPath = "/"
		}
	} else {
		return nil, errors.New("cannot determine socket path, socket file should have .sock extension")
	}

	if _, err := os.Stat(socketPath); err != nil {
		return nil, errors.New("socket file not found: ", socketPath).Base(err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}
	defer client.CloseIdleConnections()

	resp, err := client.Get("http://localhost" + httpPath)
	if err != nil {
		return nil, errors.New("failed to fetch from unix socket: ", socketPath).Base(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.New("unexpected HTTP status code: ", resp.StatusCode)
	}

	content, err := buf.ReadAllToBytes(resp.Body)
	if err != nil {
		return nil, errors.New("failed to read response").Base(err)
	}

	return content, nil
}

func init() {
	// 查看ConfigLoader的定义
	confloader.EffectiveConfigFileLoader = ConfigLoader
}
