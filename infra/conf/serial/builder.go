package serial

import (
	"context"
	"io"

	"github.com/xtls/xray-core/common/errors"
	creflect "github.com/xtls/xray-core/common/reflect"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
	"github.com/xtls/xray-core/main/confloader"
)

func MergeConfigFromFiles(files []*core.ConfigSource) (string, error) {
	// 这里返回一个合并好的*conf.Config对象
	c, err := mergeConfigs(files)
	if err != nil {
		return "", err
	}

	// 对*conf.Configl类型的对象进行序列化，跳转common/reflect/marshal.go查看MarshalToJson的定义
	// 这里为什么不直接调用json.MarshalIndent进行序列化，只知道空对象会保留，显示为null
	// if j, err := json.MarshalIndent(c, "", "  "); err == nil {
	// 	return string(j[:]), nil
	// }
	if j, ok := creflect.MarshalToJson(c, true); ok {
		return j, nil
	}
	return "", errors.New("marshal to json failed.").AtError()
}

func mergeConfigs(files []*core.ConfigSource) (*conf.Config, error) {
	cf := &conf.Config{}
	for i, file := range files {
		// commit_240623冲突，先保留下来，后面再分析
		// 跳转main/confloader/confloader.go查看LoadConfig的定义
		// 这里LoadConfig但会的r是一个实现了io.Reader接口的Buffer类型
		errors.LogInfo(context.Background(), "Reading config: ", file)
		r, err := confloader.LoadConfig(file.Name)
		if err != nil {
			return nil, errors.New("failed to read config: ", file).Base(err)
		}
		// commit_240623冲突，先保留下来，后面再分析
		// ReaderDecoderByFormat是一个map[string]readerDecoder类型，在init函数初始化，key是文件格式，value是一个对应格式的decode函数
		// 这里根据文件类型找到对应的decode函数，传入buffer调用，以json为例，跳转infra/conf/serial/loader.go查看DecodeJSONConfig的定义
		c, err := ReaderDecoderByFormat[file.Format](r)
		if err != nil {
			return nil, errors.New("failed to decode config: ", file).Base(err)
		}
		// i==0表示解析第一个文件，直接把返回的Config对象保存到cf
		if i == 0 {
			*cf = *c
			continue
		}
		// 后续每解析一个文件，都会和cf进行合并，跳转infra/conf/xray.go查看Override的定义
		cf.Override(c, file.Name)
	}
	// 返回合并好的Config对象
	return cf, nil
}

func BuildConfig(files []*core.ConfigSource) (*core.Config, error) {
	config, err := mergeConfigs(files)
	if err != nil {
		return nil, err
	}
	return config.Build()
}

type readerDecoder func(io.Reader) (*conf.Config, error)

var ReaderDecoderByFormat = make(map[string]readerDecoder)

func init() {
	// 查看各个Decode函数的定义
	ReaderDecoderByFormat["json"] = DecodeJSONConfig
	ReaderDecoderByFormat["yaml"] = DecodeYAMLConfig
	ReaderDecoderByFormat["toml"] = DecodeTOMLConfig

	core.ConfigBuilderForFiles = BuildConfig
	// 查看MergeConfigFromFiles的定义
	core.ConfigMergedFormFiles = MergeConfigFromFiles
}
