package infrastructure

import (
	"bytes"
	"strings"
	"testing"
)

func TestTOMLReadFieldValues(t *testing.T) {
	t.Run("standard table", func(t *testing.T) {
		data := []byte("model_provider = \"portico\"\n[model_providers.portico]\nname = 'portico'\nbase_url = \"http://127.0.0.1:18100\"\nwire_api = \"responses\"\nrequires_openai_auth = false\n")
		values, err := readTOMLFieldValues(data, []tomlFieldSpec{
			{Key: "provider", Path: []string{"model_provider"}},
			{Key: "name", Path: []string{"model_providers", "portico", "name"}},
			{Key: "base", Path: []string{"model_providers", "portico", "base_url"}},
			{Key: "wire", Path: []string{"model_providers", "portico", "wire_api"}},
			{Key: "auth", Path: []string{"model_providers", "portico", "requires_openai_auth"}},
			{Key: "missing", Path: []string{"model_providers", "portico", "missing"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		tomlTestAssertValue(t, values, "provider", true, `"portico"`)
		tomlTestAssertValue(t, values, "name", true, `'portico'`)
		tomlTestAssertValue(t, values, "base", true, `"http://127.0.0.1:18100"`)
		tomlTestAssertValue(t, values, "wire", true, `"responses"`)
		tomlTestAssertValue(t, values, "auth", true, "false")
		tomlTestAssertValue(t, values, "missing", false, "")
	})

	t.Run("root dotted and inline", func(t *testing.T) {
		data := []byte("model_providers.portico.name = \"dotted\"\nmodel_providers.portico.requires_openai_auth = true\nmodel_providers.inline = { name = 'inline', base_url = \"http://relay\" }\n")
		values, err := readTOMLFieldValues(data, []tomlFieldSpec{
			{Key: "dotted", Path: []string{"model_providers", "portico", "name"}},
			{Key: "auth", Path: []string{"model_providers", "portico", "requires_openai_auth"}},
			{Key: "inline", Path: []string{"model_providers", "inline", "name"}},
			{Key: "base", Path: []string{"model_providers", "inline", "base_url"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		tomlTestAssertValue(t, values, "dotted", true, `"dotted"`)
		tomlTestAssertValue(t, values, "auth", true, "true")
		tomlTestAssertValue(t, values, "inline", true, `'inline'`)
		tomlTestAssertValue(t, values, "base", true, `"http://relay"`)
	})

	t.Run("table scoped dotted keys", func(t *testing.T) {
		data := []byte("[model_providers]\nportico.name = \"dotted\"\nportico.wire_api = \"responses\"\n")
		values, err := readTOMLFieldValues(data, []tomlFieldSpec{
			{Key: "name", Path: []string{"model_providers", "portico", "name"}},
			{Key: "wire", Path: []string{"model_providers", "portico", "wire_api"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		tomlTestAssertValue(t, values, "name", true, `"dotted"`)
		tomlTestAssertValue(t, values, "wire", true, `"responses"`)
	})
}

func TestTOMLStandardTableExactEdits(t *testing.T) {
	original := []byte("# root\nmodel_provider = \"other\"\n\n[model_providers.portico] # keep table\nname = \"old\" # name comment\nunknown = 1\n\n[other]\nx = true\n")
	namePath := []string{"model_providers", "portico", "name"}
	basePath := []string{"model_providers", "portico", "base_url"}

	updated := tomlTestApply(t, original, namePath, `"new"`, true)
	tomlTestAssertBytes(t, updated, "# root\nmodel_provider = \"other\"\n\n[model_providers.portico] # keep table\nname = \"new\" # name comment\nunknown = 1\n\n[other]\nx = true\n")

	inserted := tomlTestApply(t, original, basePath, `"http://relay"`, true)
	wantInserted := "# root\nmodel_provider = \"other\"\n\n[model_providers.portico] # keep table\nname = \"old\" # name comment\nunknown = 1\nbase_url = \"http://relay\"\n\n[other]\nx = true\n"
	tomlTestAssertBytes(t, inserted, wantInserted)
	tomlTestAssertBytes(t, tomlTestApply(t, inserted, basePath, "", false), original)

	deleted := tomlTestApply(t, original, namePath, "", false)
	tomlTestAssertBytes(t, deleted, "# root\nmodel_provider = \"other\"\n\n[model_providers.portico] # keep table\nunknown = 1\n\n[other]\nx = true\n")
	tomlTestAssertBytes(t, tomlTestApply(t, original, basePath, "", false), original)
}

func TestTOMLInlineExactEditsAndRoundTrip(t *testing.T) {
	original := []byte("model_providers = { portico = { name = \"old\", other = 1 }, keep = { x = 2 } }\n")
	namePath := []string{"model_providers", "portico", "name"}
	basePath := []string{"model_providers", "portico", "base_url"}

	updated := tomlTestApply(t, original, namePath, `'new'`, true)
	tomlTestAssertBytes(t, updated, "model_providers = { portico = { name = 'new', other = 1 }, keep = { x = 2 } }\n")

	inserted := tomlTestApply(t, original, basePath, `"http://relay"`, true)
	tomlTestAssertBytes(t, inserted, "model_providers = { portico = { base_url = \"http://relay\", name = \"old\", other = 1 }, keep = { x = 2 } }\n")
	tomlTestAssertBytes(t, tomlTestApply(t, inserted, basePath, "", false), original)

	deleted := tomlTestApply(t, original, namePath, "", false)
	tomlTestAssertBytes(t, deleted, "model_providers = { portico = { other = 1 }, keep = { x = 2 } }\n")

	emptyOriginal := []byte("model_providers = {}\n")
	created := tomlTestApply(t, emptyOriginal, namePath, `"x"`, true)
	tomlTestAssertBytes(t, created, "model_providers = {portico = { name = \"x\" }}\n")
	withoutName := tomlTestApply(t, created, namePath, "", false)
	pruned := tomlTestPrune(t, withoutName, [][]string{{"model_providers", "portico"}})
	tomlTestAssertBytes(t, pruned, emptyOriginal)
}

func TestTOMLDottedExactEditsAndRoundTrip(t *testing.T) {
	t.Run("root scope", func(t *testing.T) {
		original := []byte("model_providers.portico.name = \"old\"\nmodel_providers.portico.other = 1\nmodel_provider = \"portico\"\n")
		namePath := []string{"model_providers", "portico", "name"}
		basePath := []string{"model_providers", "portico", "base_url"}

		updated := tomlTestApply(t, original, namePath, `"new"`, true)
		tomlTestAssertBytes(t, updated, "model_providers.portico.name = \"new\"\nmodel_providers.portico.other = 1\nmodel_provider = \"portico\"\n")

		inserted := tomlTestApply(t, original, basePath, `"http://relay"`, true)
		tomlTestAssertBytes(t, inserted, "model_providers.portico.name = \"old\"\nmodel_providers.portico.other = 1\nmodel_providers.portico.base_url = \"http://relay\"\nmodel_provider = \"portico\"\n")
		tomlTestAssertBytes(t, tomlTestApply(t, inserted, basePath, "", false), original)

		deleted := tomlTestApply(t, original, namePath, "", false)
		tomlTestAssertBytes(t, deleted, "model_providers.portico.other = 1\nmodel_provider = \"portico\"\n")
	})

	t.Run("standard table scope", func(t *testing.T) {
		original := []byte("[model_providers]\nportico.name = \"old\"\nportico.other = 1\n")
		basePath := []string{"model_providers", "portico", "base_url"}
		inserted := tomlTestApply(t, original, basePath, `"http://relay"`, true)
		tomlTestAssertBytes(t, inserted, "[model_providers]\nportico.name = \"old\"\nportico.other = 1\nportico.base_url = \"http://relay\"\n")
		tomlTestAssertBytes(t, tomlTestApply(t, inserted, basePath, "", false), original)
	})
}

func TestTOMLCreateParentsAndRoundTrip(t *testing.T) {
	namePath := []string{"model_providers", "portico", "name"}
	basePath := []string{"model_providers", "portico", "base_url"}

	t.Run("append standard table", func(t *testing.T) {
		original := []byte("model_provider = \"other\"\n")
		created := tomlTestApply(t, original, namePath, `"x"`, true)
		tomlTestAssertBytes(t, created, "model_provider = \"other\"\n[model_providers.portico]\nname = \"x\"\n")
		created = tomlTestApply(t, created, basePath, `"u"`, true)
		tomlTestAssertBytes(t, created, "model_provider = \"other\"\n[model_providers.portico]\nname = \"x\"\nbase_url = \"u\"\n")
		created = tomlTestApply(t, created, namePath, "", false)
		created = tomlTestApply(t, created, basePath, "", false)
		restored := tomlTestPrune(t, created, [][]string{{"model_providers", "portico"}, {"model_providers"}})
		tomlTestAssertBytes(t, restored, original)
	})

	t.Run("preserve missing final newline", func(t *testing.T) {
		original := []byte("model_provider = \"other\"")
		created := tomlTestApply(t, original, namePath, `"x"`, true)
		tomlTestAssertBytes(t, created, "model_provider = \"other\"\n[model_providers.portico]\nname = \"x\"")
		created = tomlTestApply(t, created, namePath, "", false)
		restored := tomlTestPrune(t, created, [][]string{{"model_providers", "portico"}, {"model_providers"}})
		tomlTestAssertBytes(t, restored, original)
	})

	t.Run("standard parent with dotted providers", func(t *testing.T) {
		original := []byte("[model_providers]\nother.name = \"Other\"\n")
		created := tomlTestApply(t, original, namePath, `"x"`, true)
		tomlTestAssertBytes(t, created, "[model_providers]\nother.name = \"Other\"\nportico.name = \"x\"\n")
		created = tomlTestApply(t, created, basePath, `"u"`, true)
		tomlTestAssertBytes(t, created, "[model_providers]\nother.name = \"Other\"\nportico.name = \"x\"\nportico.base_url = \"u\"\n")
	})

	t.Run("standard parent with inline providers", func(t *testing.T) {
		original := []byte("[model_providers]\nother = { name = \"Other\" }\n")
		created := tomlTestApply(t, original, namePath, `"x"`, true)
		tomlTestAssertBytes(t, created, "[model_providers]\nother = { name = \"Other\" }\nportico = { name = \"x\" }\n")
	})

	t.Run("root field on empty document", func(t *testing.T) {
		created := tomlTestApply(t, nil, []string{"model_provider"}, `"portico"`, true)
		tomlTestAssertBytes(t, created, "model_provider = \"portico\"\n")
		tomlTestAssertBytes(t, tomlTestApply(t, created, []string{"model_provider"}, "", false), "")
	})
}

func TestTOMLSemanticNoopDoesNotRewrite(t *testing.T) {
	original := []byte("[model_providers.portico]\nname = 'old'\nwire_api = \"resp\\u006fnses\"\nrequires_openai_auth = false\n")
	for _, test := range []struct {
		key string
		raw string
	}{
		{key: "name", raw: `"old"`},
		{key: "wire_api", raw: `"responses"`},
		{key: "requires_openai_auth", raw: "false"},
	} {
		t.Run(test.key, func(t *testing.T) {
			path := []string{"model_providers", "portico", test.key}
			updated := tomlTestApply(t, original, path, test.raw, true)
			tomlTestAssertBytes(t, updated, original)
		})
	}
}

func TestTOMLPruneContainers(t *testing.T) {
	t.Run("empty standard table", func(t *testing.T) {
		original := []byte("x = 1\n[model_providers.portico] # remove\n\n[other]\ny = 2\n")
		pruned := tomlTestPrune(t, original, [][]string{{"model_providers", "portico"}})
		tomlTestAssertBytes(t, pruned, "x = 1\n\n[other]\ny = 2\n")
	})

	t.Run("comments in empty table survive", func(t *testing.T) {
		original := []byte("[model_providers.portico]\n# keep\n")
		pruned := tomlTestPrune(t, original, [][]string{{"model_providers", "portico"}})
		tomlTestAssertBytes(t, pruned, "# keep\n")
	})

	t.Run("nonempty standard table unchanged", func(t *testing.T) {
		original := []byte("[model_providers.portico]\nunknown = 1\n")
		pruned := tomlTestPrune(t, original, [][]string{{"model_providers", "portico"}, {"model_providers"}})
		tomlTestAssertBytes(t, pruned, original)
	})

	t.Run("inline child and parent", func(t *testing.T) {
		original := []byte("model_providers = {portico = {}}\n")
		childPruned := tomlTestPrune(t, original, [][]string{{"model_providers", "portico"}})
		tomlTestAssertBytes(t, childPruned, "model_providers = {}\n")
		bothPruned := tomlTestPrune(t, original, [][]string{{"model_providers", "portico"}, {"model_providers"}})
		tomlTestAssertBytes(t, bothPruned, "")
	})

	t.Run("nonempty inline unchanged", func(t *testing.T) {
		original := []byte("model_providers = {portico = {name = \"x\"}}\n")
		pruned := tomlTestPrune(t, original, [][]string{{"model_providers", "portico"}, {"model_providers"}})
		tomlTestAssertBytes(t, pruned, original)
	})
}

func TestTOMLInvalidInputNeverModifiesSource(t *testing.T) {
	namePath := []string{"model_providers", "portico", "name"}
	tests := []struct {
		name string
		data string
		path []string
		raw  string
	}{
		{name: "invalid document", data: "name = [", path: namePath, raw: `"x"`},
		{name: "duplicate target", data: "[model_providers.portico]\nname = \"a\"\nname = \"b\"\n", path: namePath, raw: `"x"`},
		{name: "scalar parent", data: "model_providers = \"bad\"\n", path: namePath, raw: `"x"`},
		{name: "array parent", data: "model_providers = []\n", path: namePath, raw: `"x"`},
		{name: "extra expression in raw", data: "", path: []string{"model_provider"}, raw: "\"x\"\nevil = true"},
		{name: "trailing comment in raw", data: "", path: []string{"model_provider"}, raw: `"x" # no`},
		{name: "container target", data: "model_providers = {portico = {}}\n", path: []string{"model_providers", "portico"}, raw: `"x"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := []byte(test.data)
			before := append([]byte(nil), data...)
			if _, err := applyTOMLFieldEdit(data, test.path, test.raw, true); err == nil {
				t.Fatal("expected error")
			}
			if !bytes.Equal(data, before) {
				t.Fatalf("input modified:\nbefore %q\nafter  %q", before, data)
			}
		})
	}

	t.Run("invalid prune", func(t *testing.T) {
		data := []byte("model_providers = [")
		before := append([]byte(nil), data...)
		if _, err := pruneTOMLContainers(data, [][]string{namePath[:2]}); err == nil {
			t.Fatal("expected error")
		}
		if !bytes.Equal(data, before) {
			t.Fatal("input modified")
		}
	})

	t.Run("duplicate read target", func(t *testing.T) {
		data := []byte("[model_providers.portico]\nname = \"a\"\nname = \"b\"\n")
		if _, err := readTOMLFieldValues(data, []tomlFieldSpec{{Key: "name", Path: namePath}}); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestTOMLCRLFAndFormattingPreserved(t *testing.T) {
	original := []byte("[model_providers.portico]\r\nname = \"old\"\r\n")
	basePath := []string{"model_providers", "portico", "base_url"}
	inserted := tomlTestApply(t, original, basePath, `"u"`, true)
	tomlTestAssertBytes(t, inserted, "[model_providers.portico]\r\nname = \"old\"\r\nbase_url = \"u\"\r\n")
	tomlTestAssertBytes(t, tomlTestApply(t, inserted, basePath, "", false), original)

	spaced := []byte("model_providers . portico . name=\"old\"\n")
	inserted = tomlTestApply(t, spaced, basePath, `"u"`, true)
	tomlTestAssertBytes(t, inserted, "model_providers . portico . name=\"old\"\nmodel_providers . portico . base_url=\"u\"\n")
	if strings.Contains(string(inserted), "\r") {
		t.Fatal("unexpected CR inserted into LF document")
	}
}

func tomlTestApply(t *testing.T, data []byte, path []string, raw string, present bool) []byte {
	t.Helper()
	before := append([]byte(nil), data...)
	result, err := applyTOMLFieldEdit(data, path, raw, present)
	if err != nil {
		t.Fatalf("applyTOMLFieldEdit(%s, %q, %v): %v", formatTOMLPath(path), raw, present, err)
	}
	if !bytes.Equal(data, before) {
		t.Fatalf("input modified:\nbefore %q\nafter  %q", before, data)
	}
	return result
}

func tomlTestPrune(t *testing.T, data []byte, paths [][]string) []byte {
	t.Helper()
	before := append([]byte(nil), data...)
	result, err := pruneTOMLContainers(data, paths)
	if err != nil {
		t.Fatalf("pruneTOMLContainers: %v", err)
	}
	if !bytes.Equal(data, before) {
		t.Fatalf("input modified:\nbefore %q\nafter  %q", before, data)
	}
	return result
}

func tomlTestAssertBytes(t *testing.T, got []byte, want any) {
	t.Helper()
	var expected []byte
	switch value := want.(type) {
	case string:
		expected = []byte(value)
	case []byte:
		expected = value
	default:
		t.Fatal("invalid expected value type")
	}
	if !bytes.Equal(got, expected) {
		t.Fatalf("unexpected bytes:\n got: %q\nwant: %q", got, expected)
	}
}

func tomlTestAssertValue(t *testing.T, values map[string]tomlFieldValue, key string, present bool, raw string) {
	t.Helper()
	value, ok := values[key]
	if !ok {
		t.Fatalf("missing result %q", key)
	}
	if value.Present != present || value.Raw != raw {
		t.Fatalf("%q: got %+v, want Present=%v Raw=%q", key, value, present, raw)
	}
}
