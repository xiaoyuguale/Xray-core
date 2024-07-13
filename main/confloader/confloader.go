package confloader

import (
	"context"
	"io"
	"os"

	"github.com/xtls/xray-core/common/errors"
)

type (
	configFileLoader func(string) (io.Reader, error)
)

var (
	EffectiveConfigFileLoader configFileLoader
)

// LoadConfig reads from a path/url/stdin
// actual work is in external module
func LoadConfig(file string) (io.Reader, error) {
	// EffectiveConfigFileLoader在main/confloader/external/external.go的init函数被赋值了ConfigLoader函数，跳转查看ConfigLoader的定义
	if EffectiveConfigFileLoader == nil {
		errors.LogInfo(context.Background(), "external config module not loaded, reading from stdin")
		return os.Stdin, nil
	}
	// 调用EffectiveConfigFileLoader相当于调用ConfigLoader
	return EffectiveConfigFileLoader(file)
}
