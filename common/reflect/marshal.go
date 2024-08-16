package reflect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	cnet "github.com/xtls/xray-core/common/net"
	cserial "github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/infra/conf"
)

func MarshalToJson(v interface{}, insertTypeInfo bool) (string, bool) {
	// 查看marshalInterface的定义
	// commit_240807冲突，先保留下来，后面再分析
	// 大概看懂了marshalInterface的目的：
	// 这里的参数v，本来传入的是*conf.Config类型的变量，其实是可以直接进行json序列化的，但是这样出来的json串会包含空对象，显示为null
	// 所以这里经过了一系列的处理，把原来的*conf.Config对象转换成map（字段名由小写转换为大写），并且把null的对象都去掉了
	// 而且map转json还会按照ascii码对key进行排序，跳转标准库encoding/json/encode.go查看源码
	if itf := marshalInterface(v, true, insertTypeInfo); itf != nil {
		if b, err := JSONMarshalWithoutEscape(itf); err == nil {
			return string(b[:]), true
		}
	}
	return "", false
}

func JSONMarshalWithoutEscape(t interface{}) ([]byte, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetIndent("", "    ")
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(t)
	return buffer.Bytes(), err
}

func marshalTypedMessage(v *cserial.TypedMessage, ignoreNullValue bool, insertTypeInfo bool) interface{} {
	if v == nil {
		return nil
	}
	tmsg, err := v.GetInstance()
	if err != nil {
		return nil
	}
	r := marshalInterface(tmsg, ignoreNullValue, insertTypeInfo)
	if msg, ok := r.(map[string]interface{}); ok && insertTypeInfo {
		msg["_TypedMessage_"] = v.Type
	}
	return r
}

func marshalSlice(v reflect.Value, ignoreNullValue bool, insertTypeInfo bool) interface{} {
	r := make([]interface{}, 0)
	// 获取反射值对象的元素个数，支持Array，Chan，Map，Slice，String和指向Array的pointer
	for i := 0; i < v.Len(); i++ {
		// 获取反射值对象的第i个元素的反射值对象，支持Array，Slice和String，其他类型或者超出范围会panic
		rv := v.Index(i)
		// 元素的反射值对象可能是结构体，需要用CanInterface先判断一下
		if rv.CanInterface() {
			value := rv.Interface()
			// 再次调用marshalInterface，作用参考marshalStruct里面的分析
			r = append(r, marshalInterface(value, ignoreNullValue, insertTypeInfo))
		}
	}
	return r
}

func isNullValue(f reflect.StructField, rv reflect.Value) bool {
	if rv.Kind() == reflect.Struct {
		return false
	} else if rv.Kind() == reflect.String && rv.Len() == 0 {
		return true
	} else if !isValueKind(rv.Kind()) && rv.IsNil() {
		return true
	} else if tag := f.Tag.Get("json"); strings.Contains(tag, "omitempty") {
		if !rv.IsValid() || rv.IsZero() {
			return true
		}
	}
	return false
}

func toJsonName(f reflect.StructField) string {
	if tags := f.Tag.Get("protobuf"); len(tags) > 0 {
		for _, tag := range strings.Split(tags, ",") {
			if before, after, ok := strings.Cut(tag, "="); ok && before == "json" {
				return after
			}
		}
	}
	if tag := f.Tag.Get("json"); len(tag) > 0 {
		if before, _, ok := strings.Cut(tag, ","); ok {
			return before
		} else {
			return tag
		}
	}
	return f.Name
}

func marshalStruct(v reflect.Value, ignoreNullValue bool, insertTypeInfo bool) interface{} {
	r := make(map[string]interface{})
	// reflect.Value.Type：获取反射值对象的类型
	t := v.Type()
	// reflect.Value.NumField：从结构体的反射值对象中获取它的字段个数
	for i := 0; i < v.NumField(); i++ {
		// reflect.Value.Field：从结构体的反射值对象中获取第i个字段的反射值对象
		rv := v.Field(i)
		// reflect.Value.CanInterface：判断Value是否可以转换为接口类型
		// 如果结构体中含有不可导出字段，直接使用Value.Field(i).Interface()会panic
		// 结构体的不可导出字段不能转换为接口类型，会返回false
		if rv.CanInterface() {
			// reflect.Type.Field：返回结构体类型的第i个字段，类型是reflect.StructField
			ft := t.Field(i)
			if !ignoreNullValue || !isNullValue(ft, rv) {
				name := toJsonName(ft)
				value := rv.Interface()
				tv := marshalInterface(value, ignoreNullValue, insertTypeInfo)
				r[name] = tv
			}
		}
	}
	return r
}

func marshalMap(v reflect.Value, ignoreNullValue bool, insertTypeInfo bool) interface{} {
	// policy.level is map[uint32] *struct
	// reflect.Type.Key：获取map的key的reflect.Type，如果反射值v对象的种类不是map，则会panic
	kt := v.Type().Key()
	// 通过reflect.TypeOf构造一个reflect.Type用来表示map的值的类型，并且需要map的值允许任何值，所以只能是interface{}
	// 但是TypeOf接受nil interface时会返回nil（也就是TypeOf((interface{})(nil))的时候），所以这里需要使用*interface{}
	vt := reflect.TypeOf((*interface{})(nil))
	// reflect.MapOf：获取指定key和value类型的reflect.Type表示
	mt := reflect.MapOf(kt, vt)
	// reflect.MakeMap：获取指定reflect.Type的reflect.Value表示
	r := reflect.MakeMap(mt)
	// reflect.Value.MapKeys：获取反射值对象v的key的reflect.Value表示，以slice形式返回
	for _, key := range v.MapKeys() {
		// reflect.Value.MapIndex：相当于map[key]，不过是返回map[key]的reflect.Value的表示
		rv := v.MapIndex(key)
		// 遇到结构体，继续调用marshalInterface
		if rv.CanInterface() {
			iv := rv.Interface()
			tv := marshalInterface(iv, ignoreNullValue, insertTypeInfo)
			if tv != nil || !ignoreNullValue {
				r.SetMapIndex(key, reflect.ValueOf(&tv))
			}
		}
	}
	return r.Interface()
}

func marshalIString(v interface{}) (r string, ok bool) {
	defer func() {
		if err := recover(); err != nil {
			r = ""
			ok = false
		}
	}()
	if iStringFn, ok := v.(interface{ String() string }); ok {
		return iStringFn.String(), true
	}
	return "", false
}

func serializePortList(portList *cnet.PortList) (interface{}, bool) {
	if portList == nil {
		return nil, false
	}

	n := len(portList.Range)
	if n == 1 {
		if first := portList.Range[0]; first.From == first.To {
			return first.From, true
		}
	}

	r := make([]string, 0, n)
	for _, pr := range portList.Range {
		if pr.From == pr.To {
			r = append(r, pr.FromPort().String())
		} else {
			r = append(r, fmt.Sprintf("%d-%d", pr.From, pr.To))
		}
	}
	return strings.Join(r, ","), true
}

func marshalKnownType(v interface{}, ignoreNullValue bool, insertTypeInfo bool) (interface{}, bool) {
	// type-switch：判断某个interface变量中实际存储的变量类型
	switch ty := v.(type) {
	case cserial.TypedMessage:
		return marshalTypedMessage(&ty, ignoreNullValue, insertTypeInfo), true
	case *cserial.TypedMessage:
		return marshalTypedMessage(ty, ignoreNullValue, insertTypeInfo), true
	case map[string]json.RawMessage:
		return ty, true
	case []json.RawMessage:
		return ty, true
	case *json.RawMessage, json.RawMessage:
		return ty, true
	case *cnet.IPOrDomain:
		if domain := v.(*cnet.IPOrDomain); domain != nil {
			return domain.AsAddress().String(), true
		}
		return nil, false
	case *cnet.PortList:
		npl := v.(*cnet.PortList)
		return serializePortList(npl)
	case *conf.PortList:
		cpl := v.(*conf.PortList)
		return serializePortList(cpl.Build())
	case conf.Int32Range:
		i32rng := v.(conf.Int32Range)
		if i32rng.Left == i32rng.Right {
			return i32rng.Left, true
		}
		return i32rng.String(), true
	case cnet.Address:
		if addr := v.(cnet.Address); addr != nil {
			return addr.String(), true
		}
		return nil, false
	default:
		return nil, false
	}
}

func isValueKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.String:
		return true
	default:
		return false
	}
}

func marshalInterface(v interface{}, ignoreNullValue bool, insertTypeInfo bool) interface{} {

	// marshalKnownType内部对v进行type-switch判断，返回识别出来的类型，查看marshalKnownType的定义
	if r, ok := marshalKnownType(v, ignoreNullValue, insertTypeInfo); ok {
		return r
	}

	// 由于conf.Config对象嵌套的类型很多，marshalKnownType无法识别的类型将进行下面的处理
	// reflect.ValueOf可以获取interface的值的信息，返回reflect.Value类型的变量
	rv := reflect.ValueOf(v)
	// reflect.Value.Kind可以获取Value所属的基本种类（int，bool，pointer等等）
	if rv.Kind() == reflect.Ptr {
		// 如果Value的基本类型是指针，可以通过Elem方法获取指针指向的具体值
		rv = rv.Elem()
	}
	// 更新完rv的值，再获取kind
	k := rv.Kind()
	// 创建reflect.Value有很多方法，就算创建的时候传入的是一个无效值也不会报错
	// 如果Value的kind是Invalid，表示这个reflect.Value是无效的
	if k == reflect.Invalid {
		return nil
	}

	if ty := rv.Type().Name(); isValueKind(k) {
		if k.String() != ty {
			if s, ok := marshalIString(v); ok {
				return s
			}
		}
		return v
	}

	// fmt.Println("kind:", k, "type:", rv.Type().Name())

	// 如果k不是isValueKind指定的基本种类，是其他基本种类（struct，slice，array等），根据k的值选择不同的处理
	switch k {
	case reflect.Struct:
		// 查看marshalStruct的定义
		return marshalStruct(rv, ignoreNullValue, insertTypeInfo)
	case reflect.Slice:
		// 查看marshalSlice的定义
		return marshalSlice(rv, ignoreNullValue, insertTypeInfo)
	case reflect.Array:
		return marshalSlice(rv, ignoreNullValue, insertTypeInfo)
	case reflect.Map:
		// 查看marshalMap的定义
		return marshalMap(rv, ignoreNullValue, insertTypeInfo)
	default:
		break
	}

	if str, ok := marshalIString(v); ok {
		return str
	}
	return nil
}
