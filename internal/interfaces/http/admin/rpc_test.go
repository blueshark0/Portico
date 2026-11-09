package admin

import (
	"errors"
	"net/http"
	"reflect"
	"testing"
)

type sampleItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type SampleFacade struct {
	closed bool
	saved  []sampleItem
}

func (f *SampleFacade) Ping() string { return "pong" }

func (f *SampleFacade) Echo(name string, count int) ([]string, error) {
	out := make([]string, count)
	for i := range out {
		out[i] = name
	}
	return out, nil
}

func (f *SampleFacade) Save(item sampleItem) (sampleItem, error) {
	if item.Name == "" {
		return sampleItem{}, errors.New("name is required")
	}
	item.ID = int64(len(f.saved) + 1)
	f.saved = append(f.saved, item)
	return item, nil
}

func (f *SampleFacade) Delete(id int64) error {
	if id <= 0 {
		return errors.New("invalid id")
	}
	return nil
}

func (f *SampleFacade) Touch() {}

func (f *SampleFacade) Boom() error { panic("kaboom") }

func (f *SampleFacade) Close() error {
	f.closed = true
	return nil
}

func newSampleRegistry(t *testing.T) (*Registry, *SampleFacade) {
	t.Helper()
	facade := &SampleFacade{}
	registry, err := NewRegistry(facade)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return registry, facade
}

func wantRPCStatus(t *testing.T, err error, status int) {
	t.Helper()
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("want *RPCError with status %d, got %v", status, err)
	}
	if rpcErr.Status != status {
		t.Fatalf("want status %d, got %d (%s)", status, rpcErr.Status, rpcErr.Message)
	}
}

func TestRegistryRegistersExportedMethodsExceptDenied(t *testing.T) {
	registry, _ := newSampleRegistry(t)
	want := []string{
		"SampleFacade.Boom",
		"SampleFacade.Delete",
		"SampleFacade.Echo",
		"SampleFacade.Ping",
		"SampleFacade.Save",
		"SampleFacade.Touch",
	}
	if got := registry.Methods(); !reflect.DeepEqual(got, want) {
		t.Fatalf("methods = %v, want %v", got, want)
	}
}

func TestRegistryCloseIsNotRemotelyCallable(t *testing.T) {
	registry, facade := newSampleRegistry(t)
	_, err := registry.Call("SampleFacade.Close", nil)
	wantRPCStatus(t, err, http.StatusNotFound)
	if facade.closed {
		t.Fatal("Close must not be invoked through RPC")
	}
}

func TestRegistryCallSignatures(t *testing.T) {
	registry, _ := newSampleRegistry(t)

	if got, err := registry.Call("SampleFacade.Ping", nil); err != nil || got != "pong" {
		t.Fatalf("Ping = %v, %v", got, err)
	}
	if got, err := registry.Call("SampleFacade.Ping", []byte("  []  ")); err != nil || got != "pong" {
		t.Fatalf("Ping with empty array = %v, %v", got, err)
	}

	got, err := registry.Call("SampleFacade.Echo", []byte(`["hi", 2]`))
	if err != nil || !reflect.DeepEqual(got, []string{"hi", "hi"}) {
		t.Fatalf("Echo = %v, %v", got, err)
	}

	got, err = registry.Call("SampleFacade.Save", []byte(`[{"name":"a","extra":true}]`))
	if err != nil || got != (sampleItem{ID: 1, Name: "a"}) {
		t.Fatalf("Save = %#v, %v", got, err)
	}

	if got, err := registry.Call("SampleFacade.Delete", []byte(`[3]`)); err != nil || got != nil {
		t.Fatalf("Delete = %v, %v", got, err)
	}
	if got, err := registry.Call("SampleFacade.Touch", nil); err != nil || got != nil {
		t.Fatalf("Touch = %v, %v", got, err)
	}
}

func TestRegistryPropagatesFacadeErrors(t *testing.T) {
	registry, _ := newSampleRegistry(t)
	_, err := registry.Call("SampleFacade.Save", []byte(`[{"name":""}]`))
	if err == nil || err.Error() != "name is required" {
		t.Fatalf("want facade error, got %v", err)
	}
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		t.Fatalf("facade errors should not be wrapped as RPCError: %v", err)
	}
	if _, err := registry.Call("SampleFacade.Delete", []byte(`[0]`)); err == nil || err.Error() != "invalid id" {
		t.Fatalf("want error-only facade error, got %v", err)
	}
}

func TestRegistryRejectsBadRequests(t *testing.T) {
	registry, _ := newSampleRegistry(t)
	cases := []struct {
		name   string
		method string
		body   string
		status int
	}{
		{"unknown method", "SampleFacade.Missing", ``, http.StatusNotFound},
		{"unknown facade", "OtherFacade.Ping", ``, http.StatusNotFound},
		{"body not array", "SampleFacade.Echo", `{"name":"hi"}`, http.StatusBadRequest},
		{"malformed json", "SampleFacade.Echo", `["hi",`, http.StatusBadRequest},
		{"too few args", "SampleFacade.Echo", `["hi"]`, http.StatusBadRequest},
		{"too many args", "SampleFacade.Ping", `[1]`, http.StatusBadRequest},
		{"wrong arg type", "SampleFacade.Echo", `["hi","two"]`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := registry.Call(tc.method, []byte(tc.body))
			wantRPCStatus(t, err, tc.status)
		})
	}
}

func TestRegistryRecoversPanics(t *testing.T) {
	registry, _ := newSampleRegistry(t)
	_, err := registry.Call("SampleFacade.Boom", nil)
	wantRPCStatus(t, err, http.StatusInternalServerError)
}

type badSignatureFacade struct{}

func (badSignatureFacade) Multi() (int, int) { return 1, 2 }

type variadicFacade struct{}

func (*variadicFacade) Join(parts ...string) string { return "" }

func TestNewRegistryRejectsUnsupportedFacades(t *testing.T) {
	if _, err := NewRegistry(SampleFacade{}); err == nil {
		t.Fatal("non-pointer facade should be rejected")
	}
	var nilFacade *SampleFacade
	if _, err := NewRegistry(nilFacade); err == nil {
		t.Fatal("nil facade should be rejected")
	}
	if _, err := NewRegistry(&badSignatureFacade{}); err == nil {
		t.Fatal("(int, int) return signature should be rejected")
	}
	if _, err := NewRegistry(&variadicFacade{}); err == nil {
		t.Fatal("variadic method should be rejected")
	}
	if _, err := NewRegistry(&SampleFacade{}, &SampleFacade{}); err == nil {
		t.Fatal("duplicate facade should be rejected")
	}
}
