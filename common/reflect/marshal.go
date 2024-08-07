package reflect

import (
	"encoding/json"
	"reflect"

	cserial "github.com/xtls/xray-core/common/serial"
)

func MarshalToJson(v interface{}) (string, bool) {
	// 查看marshalInterface的定义
	// 大概看懂了marshalInterface的目的：
	// 这里的参数v，本来传入的是*conf.Config类型的变量，其实是可以直接进行json序列化的，但是这样出来的json串会包含空对象，显示为null
	// 所以这里经过了一系列的处理，把原来的*conf.Config对象转换成map（字段名由小写转换为大写），并且把null的对象都去掉了
	// 而且map转json还会按照ascii码对key进行排序，跳转标准库encoding/json/encode.go查看源码
	if itf := marshalInterface(v, true); itf != nil {
		// 这里返回的itf是一个嵌套的map
		if b, err := json.MarshalIndent(itf, "", "  "); err == nil {
			return string(b[:]), true
		}
	}
	return "", false
}

func marshalTypedMessage(v *cserial.TypedMessage, ignoreNullValue bool) interface{} {
	if v == nil {
		return nil
	}
	tmsg, err := v.GetInstance()
	if err != nil {
		return nil
	}
	r := marshalInterface(tmsg, ignoreNullValue)
	if msg, ok := r.(map[string]interface{}); ok {
		msg["_TypedMessage_"] = v.Type
	}
	return r
}

func marshalSlice(v reflect.Value, ignoreNullValue bool) interface{} {
	r := make([]interface{}, 0)
	// 获取反射值对象的元素个数，支持Array，Chan，Map，Slice，String和指向Array的pointer
	for i := 0; i < v.Len(); i++ {
		// 获取反射值对象的第i个元素的反射值对象，支持Array，Slice和String，其他类型或者超出范围会panic
		rv := v.Index(i)
		// 元素的反射值对象可能是结构体，需要用CanInterface先判断一下
		if rv.CanInterface() {
			value := rv.Interface()
			// 再次调用marshalInterface，作用参考marshalStruct里面的分析
			r = append(r, marshalInterface(value, ignoreNullValue))
		}
	}
	return r
}

func marshalStruct(v reflect.Value, ignoreNullValue bool) interface{} {
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
			// reflect.StructField.Name：返回结构体字段的名称
			name := ft.Name
			// reflect.Value.Interface：获取反射值对象的interface对象，也就是传递给reflect.ValueOf的那个变量本身
			value := rv.Interface()
			// 再次调用marshlInterface，传入interface对象（什么作用？）
			// 因为整个Config是复杂的嵌套的，需要把里面所有嵌套的子结构体也转换为map，因此要不断的调用marshalInterface
			tv := marshalInterface(value, ignoreNullValue)
			// 这里返回的tv能保存到map的条件：
			// 1. tv不为nil；
			// 2. 如果tv为nil，但是前提没有要求忽略nil（!ignoreNullValue当ignoreNullValue为false是成立），则tv可以保存到map
			if tv != nil || !ignoreNullValue {
				r[name] = tv
			}
		}
	}
	return r
}

func marshalMap(v reflect.Value, ignoreNullValue bool) interface{} {
	// policy.level is map[uint32] *struct
	kt := v.Type().Key()
	vt := reflect.TypeOf((*interface{})(nil))
	mt := reflect.MapOf(kt, vt)
	r := reflect.MakeMap(mt)
	for _, key := range v.MapKeys() {
		rv := v.MapIndex(key)
		if rv.CanInterface() {
			iv := rv.Interface()
			tv := marshalInterface(iv, ignoreNullValue)
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

func marshalKnownType(v interface{}, ignoreNullValue bool) (interface{}, bool) {
	// type-switch：判断某个interface变量中实际存储的变量类型
	switch ty := v.(type) {
	case cserial.TypedMessage:
		return marshalTypedMessage(&ty, ignoreNullValue), true
	case *cserial.TypedMessage:
		return marshalTypedMessage(ty, ignoreNullValue), true
	case map[string]json.RawMessage:
		return ty, true
	case []json.RawMessage:
		return ty, true
	case *json.RawMessage:
		return ty, true
	case json.RawMessage:
		return ty, true
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

func marshalInterface(v interface{}, ignoreNullValue bool) interface{} {

	// marshalKnownType内部对v进行type-switch判断，返回识别出来的类型，查看marshalKnownType的定义
	if r, ok := marshalKnownType(v, ignoreNullValue); ok {
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
	// 这里判断rv.Kind是否是指定的基本种类，是的话直接返回
	if isValueKind(k) {
		return v
	}

	// 如果k不是isValueKind指定的基本种类，是其他基本种类（struct，slice，array等），根据k的值选择不同的处理
	switch k {
	case reflect.Struct:
		// 查看marshalStruct的定义
		return marshalStruct(rv, ignoreNullValue)
	case reflect.Slice:
		// 查看marshalSlice的定义
		return marshalSlice(rv, ignoreNullValue)
	case reflect.Array:
		return marshalSlice(rv, ignoreNullValue)
	case reflect.Map:
		// 查看marshalMap的定义
		return marshalMap(rv, ignoreNullValue)
	default:
		break
	}

	if str, ok := marshalIString(v); ok {
		return str
	}
	return nil
}
