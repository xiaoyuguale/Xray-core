package confloader

import (
	"io"
	"os"
)

type (
	configFileLoader func(string) (io.Reader, error)
	extconfigLoader  func([]string, io.Reader) (io.Reader, error)
)

var (
	EffectiveConfigFileLoader configFileLoader
	EffectiveExtConfigLoader  extconfigLoader
)

// LoadConfig reads from a path/url/stdin
// actual work is in external module
func LoadConfig(file string) (io.Reader, error) {
	// EffectiveConfigFileLoader在main/confloader/external/external.go的init函数被赋值了ConfigLoader函数，跳转ConfigLoader的定义
	if EffectiveConfigFileLoader == nil {
		newError("external config module not loaded, reading from stdin").AtInfo().WriteToLog()
		return os.Stdin, nil
	}
	// 调用EffectiveConfigFileLoader相当于调用ConfigLoader
	return EffectiveConfigFileLoader(file)
}

// LoadExtConfig calls xctl to handle multiple config
// the actual work also in external module
func LoadExtConfig(files []string, reader io.Reader) (io.Reader, error) {
	if EffectiveExtConfigLoader == nil {
		return nil, newError("external config module not loaded").AtError()
	}

	return EffectiveExtConfigLoader(files, reader)
}
