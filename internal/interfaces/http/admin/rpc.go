package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
)

// deniedMethods 是 facade 上存在、但绝不能被远程调用的方法：生命周期方法
// （如 HotkeyFacade.Close 会关闭数据库连接）以及 Wails 服务钩子。
var deniedMethods = map[string]bool{
	"Close":           true,
	"ServiceName":     true,
	"ServiceStartup":  true,
	"ServiceShutdown": true,
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// RPCError 携带 HTTP 状态码；facade 返回的普通 error 统一按 500 处理。
type RPCError struct {
	Status  int
	Message string
}

func (e *RPCError) Error() string { return e.Message }

type rpcMethod struct {
	receiver reflect.Value
	method   reflect.Method
	params   []reflect.Type
	// returnsError / returnsValue 描述返回签名：T、error 或 (T, error)。
	returnsValue bool
	returnsError bool
}

// Registry 把 "{Facade}.{Method}" 映射到 facade 的导出方法。名称与 Wails
// 按名称生成的 bindings（"<pkg path>.{Facade}.{Method}"）的末两段一致，
// 前端 transport 只需截取后缀即可，无需维护映射表。
type Registry struct {
	methods map[string]*rpcMethod
}

// NewRegistry 通过反射登记 facade 的全部导出方法。facade 必须是指向命名结构体的指针；
// 不支持的签名（可变参数、多于两个返回值、第二个返回值不是 error）在启动时即报错。
func NewRegistry(facades ...any) (*Registry, error) {
	registry := &Registry{methods: make(map[string]*rpcMethod)}
	for _, facade := range facades {
		value := reflect.ValueOf(facade)
		typ := value.Type()
		if typ.Kind() != reflect.Pointer || typ.Elem().Kind() != reflect.Struct || value.IsNil() {
			return nil, fmt.Errorf("facade %T must be a non-nil pointer to a struct", facade)
		}
		facadeName := typ.Elem().Name()
		for i := 0; i < typ.NumMethod(); i++ {
			method := typ.Method(i)
			if deniedMethods[method.Name] {
				continue
			}
			entry, err := newRPCMethod(value, method)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", facadeName, method.Name, err)
			}
			name := facadeName + "." + method.Name
			if _, exists := registry.methods[name]; exists {
				return nil, fmt.Errorf("duplicate rpc method %s", name)
			}
			registry.methods[name] = entry
		}
	}
	return registry, nil
}

func newRPCMethod(receiver reflect.Value, method reflect.Method) (*rpcMethod, error) {
	fnType := method.Type
	if fnType.IsVariadic() {
		return nil, errors.New("variadic methods are not supported")
	}
	entry := &rpcMethod{receiver: receiver, method: method}
	// In(0) 是接收者。
	for i := 1; i < fnType.NumIn(); i++ {
		entry.params = append(entry.params, fnType.In(i))
	}
	switch fnType.NumOut() {
	case 0:
	case 1:
		if fnType.Out(0) == errorType {
			entry.returnsError = true
		} else {
			entry.returnsValue = true
		}
	case 2:
		if fnType.Out(1) != errorType {
			return nil, errors.New("second return value must be error")
		}
		entry.returnsValue = true
		entry.returnsError = true
	default:
		return nil, errors.New("at most two return values are supported")
	}
	return entry, nil
}

// Methods 返回已登记的方法名（排序后），用于日志与测试。
func (r *Registry) Methods() []string {
	names := make([]string, 0, len(r.methods))
	for name := range r.methods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Call 以 JSON 数组形式的参数调用方法；空 body 视为无参数。
func (r *Registry) Call(name string, rawArgs []byte) (result any, err error) {
	entry, ok := r.methods[name]
	if !ok {
		return nil, &RPCError{Status: http.StatusNotFound, Message: fmt.Sprintf("unknown method %q", name)}
	}

	var args []json.RawMessage
	if trimmed := bytes.TrimSpace(rawArgs); len(trimmed) > 0 {
		if err := json.Unmarshal(trimmed, &args); err != nil {
			return nil, &RPCError{Status: http.StatusBadRequest, Message: "request body must be a JSON array of arguments"}
		}
	}
	if len(args) != len(entry.params) {
		return nil, &RPCError{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("%s expects %d arguments, got %d", name, len(entry.params), len(args)),
		}
	}

	in := make([]reflect.Value, 0, len(args)+1)
	in = append(in, entry.receiver)
	for i, paramType := range entry.params {
		arg := reflect.New(paramType)
		if err := json.Unmarshal(args[i], arg.Interface()); err != nil {
			return nil, &RPCError{
				Status:  http.StatusBadRequest,
				Message: fmt.Sprintf("%s argument %d: %v", name, i+1, err),
			}
		}
		in = append(in, arg.Elem())
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = &RPCError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("%s panicked: %v", name, recovered)}
		}
	}()
	out := entry.method.Func.Call(in)

	if entry.returnsError {
		if errValue := out[len(out)-1]; !errValue.IsNil() {
			return nil, errValue.Interface().(error)
		}
	}
	if entry.returnsValue {
		return out[0].Interface(), nil
	}
	return nil, nil
}
