package infrastructure

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestCodexConcurrentProxyAndMCPOperations(t *testing.T) {
	m := newCodexManager(t)
	writeTestConfig(t, codexConfigPath(t), []byte("model = \"keep\"\n"))
	writeTestConfig(t, codexAuthPathForTest(t), []byte(`{"tokens":{"access":"a"}}`))

	const iterations = 10
	var wg sync.WaitGroup
	errorsCh := make(chan error, iterations*4)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := m.EnableProxy(); err != nil {
				errorsCh <- fmt.Errorf("Enable: %w", err)
				return
			}
			if err := m.DisableProxy(); err != nil {
				errorsCh <- fmt.Errorf("Disable: %w", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := m.InjectMcp(); err != nil {
				errorsCh <- fmt.Errorf("InjectMcp: %w", err)
				return
			}
			if err := m.RemoveMcp(); err != nil {
				errorsCh <- fmt.Errorf("RemoveMcp: %w", err)
				return
			}
		}
	}()
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Error(err)
	}
	if t.Failed() {
		t.FailNow()
	}

	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if err := m.InjectMcp(); err != nil {
		t.Fatal(err)
	}
	config := readTestConfig(t, codexConfigPath(t))
	var raw map[string]any
	if err := toml.Unmarshal(config, &raw); err != nil {
		t.Fatalf("并发操作后 TOML 必须有效: %v\n%s", err, config)
	}
	if ok, err := m.McpStatus(); err != nil || !ok {
		t.Fatalf("最终 MCP 条目应可用: %v, %v", ok, err)
	}
	auth := readTestConfig(t, codexAuthPathForTest(t))
	if !json.Valid(auth) {
		t.Fatalf("并发操作后 auth.json 必须有效: %s", auth)
	}
}
