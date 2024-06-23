package serial

import (
	"io"

	creflect "github.com/xtls/xray-core/common/reflect"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
	"github.com/xtls/xray-core/main/confloader"
)

func MergeConfigFromFiles(files []string, formats []string) (string, error) {
	c, err := mergeConfigs(files, formats)
	if err != nil {
		return "", err
	}

	if j, ok := creflect.MarshalToJson(c); ok {
		return j, nil
	}
	return "", newError("marshal to json failed.").AtError()
}

func mergeConfigs(files []string, formats []string) (*conf.Config, error) {
	cf := &conf.Config{}
	for i, file := range files {
		newError("Reading config: ", file).AtInfo().WriteToLog()
		// 跳转main/confloader/confloader.go查看LoadConfig的定义
		// 这里LoadConfig但会的r是一个实现了io.Reader接口的Buffer类型
		r, err := confloader.LoadConfig(file)
		if err != nil {
			return nil, newError("failed to read config: ", file).Base(err)
		}
		// ReaderDecoderByFormat是一个map类型，在init函数初始化，key是文件格式，value是一个对应格式的decode函数
		// 这里根据文件类型找到对应的decode函数，传入buffer调用，以json为例，跳转infra/conf/serial/loader.go查看DecodeJSONConfig的定义
		c, err := ReaderDecoderByFormat[formats[i]](r)
		if err != nil {
			return nil, newError("failed to decode config: ", file).Base(err)
		}
		if i == 0 {
			*cf = *c
			continue
		}
		cf.Override(c, file)
	}
	return cf, nil
}

func BuildConfig(files []string, formats []string) (*core.Config, error) {
	config, err := mergeConfigs(files, formats)
	if err != nil {
		return nil, err
	}
	return config.Build()
}

type readerDecoder func(io.Reader) (*conf.Config, error)

var ReaderDecoderByFormat = make(map[string]readerDecoder)

func init() {
	ReaderDecoderByFormat["json"] = DecodeJSONConfig
	ReaderDecoderByFormat["yaml"] = DecodeYAMLConfig
	ReaderDecoderByFormat["toml"] = DecodeTOMLConfig

	core.ConfigBuilderForFiles = BuildConfig
	// 查看MergeConfigFromFiles的定义
	core.ConfigMergedFormFiles = MergeConfigFromFiles
}
