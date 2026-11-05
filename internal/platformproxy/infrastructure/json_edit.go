package infrastructure

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type jsonFieldEdit struct {
	Path    string
	Raw     string
	Present bool
}

// readJSONFieldValues 只读取原始值，不对配置做 parse→marshal 往返。
// nil slice 表示字段原本不存在；单元素 slice 保留其 raw JSON。
func readJSONFieldValues(data []byte, paths []string) (map[string][]string, error) {
	if err := validateJSONObject(data); err != nil {
		return nil, err
	}
	values := make(map[string][]string, len(paths))
	for _, path := range paths {
		if err := validateJSONParents(data, path); err != nil {
			return nil, err
		}
		result := gjson.GetBytes(data, path)
		if result.Exists() {
			values[path] = []string{result.Raw}
		} else {
			values[path] = nil
		}
	}
	return values, nil
}

func jsonSnapshotEdits(fields map[string][]string, paths []string) ([]jsonFieldEdit, error) {
	edits := make([]jsonFieldEdit, 0, len(paths))
	for _, path := range paths {
		values := fields[path]
		if len(values) > 1 {
			return nil, fmt.Errorf("JSON 字段 %s 的快照含有多个值", path)
		}
		edit := jsonFieldEdit{Path: path, Present: len(values) == 1}
		if edit.Present {
			edit.Raw = values[0]
			if !json.Valid([]byte(edit.Raw)) {
				return nil, fmt.Errorf("JSON 字段 %s 的快照不是合法 JSON", path)
			}
		}
		edits = append(edits, edit)
	}
	return edits, nil
}

func jsonContainerPresence(data []byte, paths []string) (map[string]bool, error) {
	if err := validateJSONObject(data); err != nil {
		return nil, err
	}
	presence := make(map[string]bool, len(paths))
	for _, path := range paths {
		result := gjson.GetBytes(data, path)
		if result.Exists() && !result.IsObject() {
			return nil, fmt.Errorf("JSON 字段 %s 不是对象，拒绝覆盖", path)
		}
		presence[path] = result.Exists()
	}
	return presence, nil
}

// applyJSONFieldEdits 按顺序做最小编辑。sjson 对已有值采用原文字节拼接，
// 因而不会重排其他键或重建未知对象。
func applyJSONFieldEdits(data []byte, edits []jsonFieldEdit) ([]byte, error) {
	if err := validateJSONObject(data); err != nil {
		return nil, err
	}
	current := data
	for _, edit := range edits {
		if err := validateJSONParents(current, edit.Path); err != nil {
			return nil, err
		}
		existing := gjson.GetBytes(current, edit.Path)
		if !edit.Present {
			if existing.Exists() {
				updated, err := sjson.DeleteBytes(current, edit.Path)
				if err != nil {
					return nil, fmt.Errorf("删除 JSON 字段 %s 失败: %w", edit.Path, err)
				}
				current = updated
			}
			continue
		}
		if !json.Valid([]byte(edit.Raw)) {
			return nil, fmt.Errorf("JSON 字段 %s 的写入值不是合法 JSON", edit.Path)
		}
		if existing.Exists() && jsonRawValuesEqual(existing.Raw, edit.Raw) {
			continue
		}
		updated, err := sjson.SetRawBytes(current, edit.Path, []byte(edit.Raw))
		if err != nil {
			return nil, fmt.Errorf("修改 JSON 字段 %s 失败: %w", edit.Path, err)
		}
		current = updated
	}
	if err := validateJSONObject(current); err != nil {
		return nil, fmt.Errorf("JSON 字段补丁校验失败: %w", err)
	}
	return current, nil
}

// pruneJSONContainers 只删除为空的容器；有用户内容的容器保持不动。
func pruneJSONContainers(data []byte, paths []string) ([]byte, error) {
	current := data
	for _, path := range paths {
		result := gjson.GetBytes(current, path)
		if !result.Exists() {
			continue
		}
		if !result.IsObject() {
			return nil, fmt.Errorf("JSON 字段 %s 不是对象，拒绝清理", path)
		}
		if len(result.Map()) != 0 {
			continue
		}
		updated, err := sjson.DeleteBytes(current, path)
		if err != nil {
			return nil, fmt.Errorf("清理空 JSON 对象 %s 失败: %w", path, err)
		}
		current = updated
	}
	if err := validateJSONObject(current); err != nil {
		return nil, err
	}
	return current, nil
}

func jsonRootIsEmpty(data []byte) (bool, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return false, err
	}
	if root == nil {
		return false, errors.New("JSON 配置根节点必须是对象")
	}
	return len(root) == 0, nil
}

func validateJSONObject(data []byte) error {
	if !json.Valid(data) {
		return errors.New("JSON 配置损坏，拒绝覆盖")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("JSON 配置根节点必须是对象: %w", err)
	}
	if root == nil {
		return errors.New("JSON 配置根节点必须是对象")
	}
	return nil
}

func validateJSONParents(data []byte, path string) error {
	for index := strings.LastIndex(path, "."); index >= 0; index = strings.LastIndex(path[:index], ".") {
		parent := path[:index]
		result := gjson.GetBytes(data, parent)
		if result.Exists() && !result.IsObject() {
			return fmt.Errorf("JSON 字段 %s 的父节点 %s 不是对象，拒绝覆盖", path, parent)
		}
	}
	return nil
}

func jsonRawValuesEqual(left, right string) bool {
	leftValue, leftErr := decodeJSONRaw(left)
	rightValue, rightErr := decodeJSONRaw(right)
	if leftErr != nil || rightErr != nil {
		return left == right
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func decodeJSONRaw(raw string) (any, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON raw value 含有额外内容")
	}
	return value, nil
}
