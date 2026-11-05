package infrastructure

import (
	"strings"
	"testing"
)

func TestJSONFieldEditsPreserveStructureAndOrder(t *testing.T) {
	input := []byte(`{
  "z": 1,
  "env": {
    "OTHER": "keep",
    "ANTHROPIC_AUTH_TOKEN": "old",
    "ANTHROPIC_BASE_URL": "http://old"
  },
  "a": {"nested": true}
}`)
	updated, err := applyJSONFieldEdits(input, []jsonFieldEdit{
		{Path: "env.ANTHROPIC_AUTH_TOKEN", Raw: `"portico"`, Present: true},
		{Path: "env.ANTHROPIC_BASE_URL", Raw: `"http://127.0.0.1:18100"`, Present: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.Replace(string(input), `"old"`, `"portico"`, 1)
	expected = strings.Replace(expected, `"http://old"`, `"http://127.0.0.1:18100"`, 1)
	if string(updated) != expected {
		t.Fatalf("除目标值外不应改变任何字节:\n--- got ---\n%s\n--- want ---\n%s", updated, expected)
	}
}

func TestJSONFieldEditsInsertAndDeleteWithoutReordering(t *testing.T) {
	input := []byte(`{"before":1,"env":{"keep":2},"after":3}`)
	updated, err := applyJSONFieldEdits(input, []jsonFieldEdit{
		{Path: "env.ANTHROPIC_AUTH_TOKEN", Raw: `"portico"`, Present: true},
		{Path: "env.ANTHROPIC_BASE_URL", Raw: `"url"`, Present: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"before":1,"env":{"keep":2,"ANTHROPIC_AUTH_TOKEN":"portico","ANTHROPIC_BASE_URL":"url"},"after":3}`
	if string(updated) != expected {
		t.Fatalf("插入不应改变已有结构: got %s want %s", updated, expected)
	}
	restored, err := applyJSONFieldEdits(updated, []jsonFieldEdit{
		{Path: "env.ANTHROPIC_AUTH_TOKEN", Present: false},
		{Path: "env.ANTHROPIC_BASE_URL", Present: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(input) {
		t.Fatalf("删除新增字段后应恢复原字节: got %s want %s", restored, input)
	}
}

func TestJSONFieldEditsSemanticEqualKeepsRawRepresentation(t *testing.T) {
	input := []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"portico"}}`)
	updated, err := applyJSONFieldEdits(input, []jsonFieldEdit{
		{Path: "env.ANTHROPIC_AUTH_TOKEN", Raw: `"portico"`, Present: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(updated) != string(input) {
		t.Fatalf("语义相同的字符串不应重写原始表示: got %s want %s", updated, input)
	}
}

func TestJSONFieldSnapshotDistinguishesNullAndMissing(t *testing.T) {
	values, err := readJSONFieldValues([]byte(`{"env":{"present":null}}`), []string{"env.present", "env.missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(values["env.present"]) != 1 || values["env.present"][0] != "null" {
		t.Fatalf("null 应作为存在的 raw 值保存: %#v", values)
	}
	if values["env.missing"] != nil {
		t.Fatalf("缺失字段应为 nil: %#v", values["env.missing"])
	}
}

func TestPruneJSONContainersOnlyRemovesEmpty(t *testing.T) {
	input := []byte(`{"before":1,"empty":{},"nonempty":{"keep":2},"after":3}`)
	updated, err := pruneJSONContainers(input, []string{"empty", "nonempty"})
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"before":1,"nonempty":{"keep":2},"after":3}`
	if string(updated) != expected {
		t.Fatalf("只应删除空对象: got %s want %s", updated, expected)
	}
}

func TestJSONFieldEditsRejectInvalidParentWithoutChangingInput(t *testing.T) {
	input := []byte(`{"env":"do-not-replace","keep":1}`)
	updated, err := applyJSONFieldEdits(input, []jsonFieldEdit{{Path: "env.KEY", Raw: `"x"`, Present: true}})
	if err == nil {
		t.Fatal("父节点不是对象时应报错")
	}
	if updated != nil || string(input) != `{"env":"do-not-replace","keep":1}` {
		t.Fatalf("失败不得修改原输入: %s", updated)
	}
}
