package platform // import "github.com/xtls/xray-core/common/platform"

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	ConfigLocation  = "xray.location.config"
	ConfdirLocation = "xray.location.confdir"
	AssetLocation   = "xray.location.asset"
	CertLocation    = "xray.location.cert"

	UseReadV         = "xray.buf.readv"
	UseFreedomSplice = "xray.buf.splice"
	UseVmessPadding  = "xray.vmess.padding"
	UseCone          = "xray.cone.disabled"

	BufferSize           = "xray.ray.buffer.size"
	BrowserDialerAddress = "xray.browser.dialer"
	XUDPLog              = "xray.xudp.show"
	XUDPBaseKey          = "xray.xudp.basekey"

	TunFdKey = "xray.tun.fd"

	MphCachePath = "xray.mph.cache"
)

type EnvFlag struct {
	Name    string
	AltName string
}

func NewEnvFlag(name string) EnvFlag {
	return EnvFlag{
		Name:    name,
		AltName: NormalizeEnvName(name), // 使用strings方法处理name，查看NormalizeEnvName的定义
	}
}

func (f EnvFlag) GetValue(defaultValue func() string) string {
	// os.LookupEnv：查找环境变量，如果环境变量存在，返回环境变量的值和true，否则返回空字符串和false
	if v, found := os.LookupEnv(f.Name); found {
		return v
	}
	if len(f.AltName) > 0 {
		if v, found := os.LookupEnv(f.AltName); found {
			return v
		}
	}

	// 返回defaultValue()的结果，根据GetConfDirPath()传入的结果，这里应该是返回空字符串
	return defaultValue()
}

func (f EnvFlag) GetValueAsInt(defaultValue int) int {
	useDefaultValue := false
	s := f.GetValue(func() string {
		useDefaultValue = true
		return ""
	})
	if useDefaultValue {
		return defaultValue
	}
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return defaultValue
	}
	return int(v)
}

func NormalizeEnvName(name string) string {
	// strings.TrimSpace：删除字符串s开头和结尾的空白字符，不包括中间的空白字符，类似Python的strip()
	// strings.ToUpper：将字符串s的所有字符修改为大写形式
	// strings.ReplaceAll：将字符串s中的old字符串全部替换为new字符串
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(name)), ".", "_")
}

func getExecutableDir() string {
	exec, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exec)
}

func GetConfigurationPath() string {
	configPath := NewEnvFlag(ConfigLocation).GetValue(getExecutableDir)
	return filepath.Join(configPath, "config.json")
}

// GetConfDirPath reads "xray.location.confdir"
func GetConfDirPath() string {
	// ConfdirLocation是一个常量，通过GetValue获取环境变量指向的路径，查看GetValue的定义
	configPath := NewEnvFlag(ConfdirLocation).GetValue(func() string { return "" })
	return configPath
}
