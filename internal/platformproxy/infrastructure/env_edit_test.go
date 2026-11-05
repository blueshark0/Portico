package infrastructure

import (
	"strings"
	"testing"
)

func TestEnvSetFieldPreservesLinesCommentsAndOrder(t *testing.T) {
	input := []byte("# heading\nOTHER=keep\nexport GEMINI_API_KEY = \"old\" # key comment\n\nGOOGLE_GEMINI_BASE_URL='http://old'\nTAIL = unchanged\n")
	updated, err := setEnvField(input, "GEMINI_API_KEY", "portico")
	if err != nil {
		t.Fatal(err)
	}
	updated, err = setEnvField(updated, "GOOGLE_GEMINI_BASE_URL", "http://127.0.0.1:18100/gemini")
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.Replace(string(input), `"old"`, "portico", 1)
	expected = strings.Replace(expected, `'http://old'`, "http://127.0.0.1:18100/gemini", 1)
	if string(updated) != expected {
		t.Fatalf("只应替换值区间:\n--- got ---\n%s\n--- want ---\n%s", updated, expected)
	}
}

func TestEnvInsertThenRestoreMissingKeysRoundtrip(t *testing.T) {
	input := []byte("# keep\nA=1\n")
	updated, err := setEnvField(input, "GEMINI_API_KEY", "portico")
	if err != nil {
		t.Fatal(err)
	}
	updated, err = setEnvField(updated, "GOOGLE_GEMINI_BASE_URL", "url")
	if err != nil {
		t.Fatal(err)
	}
	expected := "# keep\nA=1\nGEMINI_API_KEY=portico\nGOOGLE_GEMINI_BASE_URL=url"
	if string(updated) != expected {
		t.Fatalf("追加字段不应改变已有行: got %q want %q", updated, expected)
	}
	restored, err := restoreEnvField(updated, "GEMINI_API_KEY", nil)
	if err != nil {
		t.Fatal(err)
	}
	restored, err = restoreEnvField(restored, "GOOGLE_GEMINI_BASE_URL", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(input) {
		t.Fatalf("删除新增字段后应恢复原文件: got %q want %q", restored, input)
	}
}

func TestEnvDuplicateKeysSnapshotAndRestore(t *testing.T) {
	input := []byte("GEMINI_API_KEY=first\nOTHER=1\nexport GEMINI_API_KEY=\"second\" # keep\n")
	values, err := readEnvFieldValues(input, []string{"GEMINI_API_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	if len(values["GEMINI_API_KEY"]) != 2 {
		t.Fatalf("两个原始赋值都应保存: %#v", values)
	}
	updated, err := setEnvField(input, "GEMINI_API_KEY", "portico")
	if err != nil {
		t.Fatal(err)
	}
	effective, err := envEffectiveValues(updated, []string{"GEMINI_API_KEY"})
	if err != nil || effective["GEMINI_API_KEY"] != "portico" {
		t.Fatalf("所有重复赋值都应更新: %#v, %v", effective, err)
	}
	restored, err := restoreEnvField(updated, "GEMINI_API_KEY", values["GEMINI_API_KEY"])
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(input) {
		t.Fatalf("重复 key 应按原行恢复: got %q want %q", restored, input)
	}
}

func TestEnvRestoreChangedDuplicateCountKeepsOtherLines(t *testing.T) {
	original := []byte("GEMINI_API_KEY=original\n")
	values, err := readEnvFieldValues(original, []string{"GEMINI_API_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	current := []byte("BEFORE=1\nGEMINI_API_KEY=one\nMIDDLE=2\nGEMINI_API_KEY=two\nAFTER=3\n")
	restored, err := restoreEnvField(current, "GEMINI_API_KEY", values["GEMINI_API_KEY"])
	if err != nil {
		t.Fatal(err)
	}
	expected := "BEFORE=1\nGEMINI_API_KEY=original\nMIDDLE=2\nAFTER=3\n"
	if string(restored) != expected {
		t.Fatalf("数量变化时仍应保留其他行顺序: got %q want %q", restored, expected)
	}
}

func TestEnvSemanticEqualKeepsQuotesAndCRLF(t *testing.T) {
	input := []byte("export GEMINI_API_KEY = \"portico\" # keep\r\nOTHER=1\r\n")
	updated, err := setEnvField(input, "GEMINI_API_KEY", "portico")
	if err != nil {
		t.Fatal(err)
	}
	if string(updated) != string(input) {
		t.Fatalf("语义相同不应改变引号或 CRLF: got %q want %q", updated, input)
	}
}

func TestEnvMalformedUnrelatedLineDoesNotBlockManagedField(t *testing.T) {
	input := []byte("OTHER=\"unterminated\nGEMINI_API_KEY=old\n")
	updated, err := setEnvField(input, "GEMINI_API_KEY", "portico")
	if err != nil {
		t.Fatal(err)
	}
	expected := "OTHER=\"unterminated\nGEMINI_API_KEY=portico\n"
	if string(updated) != expected {
		t.Fatalf("未识别其他行应原样保留: got %q want %q", updated, expected)
	}
}

func TestEnvMalformedManagedLineReturnsError(t *testing.T) {
	input := []byte("GEMINI_API_KEY=\"unterminated\n")
	if _, err := setEnvField(input, "GEMINI_API_KEY", "portico"); err == nil {
		t.Fatal("受管字段引号未闭合时应报错")
	}
	if string(input) != "GEMINI_API_KEY=\"unterminated\n" {
		t.Fatalf("失败不得修改输入: %q", input)
	}
}
