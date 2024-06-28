package serial

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/ghodss/yaml"
	"github.com/pelletier/go-toml"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
	json_reader "github.com/xtls/xray-core/infra/conf/json"
)

type offset struct {
	line int
	char int
}

func findOffset(b []byte, o int) *offset {
	if o >= len(b) || o < 0 {
		return nil
	}

	line := 1
	char := 0
	for i, x := range b {
		if i == o {
			break
		}
		if x == '\n' {
			line++
			char = 0
		} else {
			char++
		}
	}

	return &offset{line: line, char: char}
}

// DecodeJSONConfig reads from reader and decode the config into *conf.Config
// syntax error could be detected.
func DecodeJSONConfig(reader io.Reader) (*conf.Config, error) {
	jsonConfig := &conf.Config{}

	// 创建指定容量的buffer，默认是64
	jsonContent := bytes.NewBuffer(make([]byte, 0, 10240))
	// io.TeeReader：传入一个Reader和一个Writer，返回一个teeReader对象，当读取teeReader对象的内容时，会无缓冲的将读取内容写到Writer中
	// 这里写入jsonContent主要是为了反序列化出错的时候，获取到出错的字符和行数
	jsonReader := io.TeeReader(&json_reader.Reader{
		Reader: reader,
	}, jsonContent)
	decoder := json.NewDecoder(jsonReader)

	// 把config数据流反序列化到变量jsonConfig，jsonConfig是一个*conf.Config结构体类型的变量，跳转infra/conf/xray.go查看结构体Config的定义
	if err := decoder.Decode(jsonConfig); err != nil {
		var pos *offset
		cause := errors.Cause(err)
		switch tErr := cause.(type) {
		case *json.SyntaxError:
			pos = findOffset(jsonContent.Bytes(), int(tErr.Offset))
		case *json.UnmarshalTypeError:
			pos = findOffset(jsonContent.Bytes(), int(tErr.Offset))
		}
		if pos != nil {
			return nil, errors.New("failed to read config file at line ", pos.line, " char ", pos.char).Base(err)
		}
		return nil, errors.New("failed to read config file").Base(err)
	}

	// 返回反序列化成功的*conf.Config类型的结构体
	return jsonConfig, nil
}

func LoadJSONConfig(reader io.Reader) (*core.Config, error) {
	jsonConfig, err := DecodeJSONConfig(reader)
	if err != nil {
		return nil, err
	}

	pbConfig, err := jsonConfig.Build()
	if err != nil {
		return nil, errors.New("failed to parse json config").Base(err)
	}

	return pbConfig, nil
}

// DecodeTOMLConfig reads from reader and decode the config into *conf.Config
// using github.com/pelletier/go-toml and map to convert toml to json.
func DecodeTOMLConfig(reader io.Reader) (*conf.Config, error) {
	tomlFile, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.New("failed to read config file").Base(err)
	}

	configMap := make(map[string]interface{})
	if err := toml.Unmarshal(tomlFile, &configMap); err != nil {
		return nil, errors.New("failed to convert toml to map").Base(err)
	}

	jsonFile, err := json.Marshal(&configMap)
	if err != nil {
		return nil, errors.New("failed to convert map to json").Base(err)
	}

	return DecodeJSONConfig(bytes.NewReader(jsonFile))
}

func LoadTOMLConfig(reader io.Reader) (*core.Config, error) {
	tomlConfig, err := DecodeTOMLConfig(reader)
	if err != nil {
		return nil, err
	}

	pbConfig, err := tomlConfig.Build()
	if err != nil {
		return nil, errors.New("failed to parse toml config").Base(err)
	}

	return pbConfig, nil
}

// DecodeYAMLConfig reads from reader and decode the config into *conf.Config
// using github.com/ghodss/yaml to convert yaml to json.
func DecodeYAMLConfig(reader io.Reader) (*conf.Config, error) {
	yamlFile, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.New("failed to read config file").Base(err)
	}

	jsonFile, err := yaml.YAMLToJSON(yamlFile)
	if err != nil {
		return nil, errors.New("failed to convert yaml to json").Base(err)
	}

	return DecodeJSONConfig(bytes.NewReader(jsonFile))
}

func LoadYAMLConfig(reader io.Reader) (*core.Config, error) {
	yamlConfig, err := DecodeYAMLConfig(reader)
	if err != nil {
		return nil, err
	}

	pbConfig, err := yamlConfig.Build()
	if err != nil {
		return nil, errors.New("failed to parse yaml config").Base(err)
	}

	return pbConfig, nil
}
