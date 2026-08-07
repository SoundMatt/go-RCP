//fusa:test REQ-CFG-001
//fusa:test REQ-CFG-002
//fusa:test REQ-CFG-003
//fusa:test REQ-CFG-004
//fusa:test REQ-CFG-005
//fusa:test REQ-CFG-006
//fusa:test REQ-CFG-007
//fusa:test REQ-CFG-008

package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/config"
	"github.com/SoundMatt/go-RCP/v9/regmap"
)

const yamlConfig = `
version: 1
servers:
  - key: front-left
    stream_id: "0211223344550001"
    transport: udp
    address: "192.168.1.10:5000"
    endpoints:
      - address: 1
        type: 1
  - key: front-right
    stream_id: "0211223344550002"
    transport: tls
    address: "192.168.1.11:5001"
    cert_file: "/etc/rcp/fr.crt"
    key_file:  "/etc/rcp/fr.key"
    ca_file:   "/etc/rcp/ca.crt"
`

const jsonConfig = `{
  "version": 1,
  "servers": [
    {"key": "a", "stream_id": "0211223344550001", "transport": "udp", "address": "10.0.0.1:5000"},
    {"key": "b", "stream_id": "0211223344550003", "transport": "mock", "address": ""}
  ]
}`

func TestDecode_YAML(t *testing.T) {
	cfg, err := config.Decode(strings.NewReader(yamlConfig), ".yaml")
	if err != nil {
		t.Fatalf("Decode YAML: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("version = %d, want 1", cfg.Version)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("want 2 servers, got %d", len(cfg.Servers))
	}
	if cfg.Servers[0].Key != "front-left" {
		t.Errorf("servers[0].Key = %q, want front-left", cfg.Servers[0].Key)
	}
	if cfg.Servers[0].Transport != config.TransportUDP {
		t.Errorf("transport = %q, want udp", cfg.Servers[0].Transport)
	}
	if len(cfg.Servers[0].Endpoints) != 1 || cfg.Servers[0].Endpoints[0].Type != regmap.EndpointTypeGPIO {
		t.Errorf("endpoints = %+v, want one GPIO endpoint", cfg.Servers[0].Endpoints)
	}
	id, err := cfg.Servers[0].DecodeStreamID()
	if err != nil {
		t.Fatalf("DecodeStreamID: %v", err)
	}
	if id.String() == "" {
		t.Error("DecodeStreamID: unexpectedly empty")
	}
}

func TestDecode_JSON(t *testing.T) {
	cfg, err := config.Decode(strings.NewReader(jsonConfig), ".json")
	if err != nil {
		t.Fatalf("Decode JSON: %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("want 2 servers, got %d", len(cfg.Servers))
	}
}

func TestDecode_YAML_Alias(t *testing.T) {
	cfg, err := config.Decode(strings.NewReader(yamlConfig), ".yml")
	if err != nil {
		t.Fatalf("Decode .yml: %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Errorf("want 2 servers, got %d", len(cfg.Servers))
	}
}

func TestDecode_UnsupportedExtension(t *testing.T) {
	_, err := config.Decode(strings.NewReader("{}"), ".toml")
	if err == nil {
		t.Error("want error for unsupported extension")
	}
}

func TestDecode_InvalidVersion(t *testing.T) {
	const bad = `{"version": 99, "servers": []}`
	_, err := config.Decode(strings.NewReader(bad), ".json")
	if !errors.Is(err, config.ErrInvalidVersion) {
		t.Errorf("want ErrInvalidVersion, got %v", err)
	}
}

func TestDecode_DuplicateServer(t *testing.T) {
	const dup = `{"version": 1, "servers": [
		{"key": "a", "stream_id": "0211223344550001", "transport": "mock"},
		{"key": "a", "stream_id": "0211223344550002", "transport": "udp"}
	]}`
	_, err := config.Decode(strings.NewReader(dup), ".json")
	if !errors.Is(err, config.ErrDuplicateServer) {
		t.Errorf("want ErrDuplicateServer, got %v", err)
	}
}

func TestDecode_InvalidStreamID(t *testing.T) {
	const bad = `{"version": 1, "servers": [
		{"key": "a", "stream_id": "not-hex", "transport": "mock"}
	]}`
	_, err := config.Decode(strings.NewReader(bad), ".json")
	if !errors.Is(err, config.ErrInvalidStreamID) {
		t.Errorf("want ErrInvalidStreamID, got %v", err)
	}
}

func TestDecode_DuplicateEndpointAddress(t *testing.T) {
	const dup = `{"version": 1, "servers": [
		{"key": "a", "stream_id": "0211223344550001", "transport": "mock", "endpoints": [
			{"address": 1, "type": 1},
			{"address": 1, "type": 2}
		]}
	]}`
	_, err := config.Decode(strings.NewReader(dup), ".json")
	if !errors.Is(err, config.ErrDuplicateAddress) {
		t.Errorf("want ErrDuplicateAddress, got %v", err)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("want error for missing file")
	}
}

func TestLoad_ValidFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "servers.yaml")
	if err := os.WriteFile(f, []byte(yamlConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Errorf("want 2 servers, got %d", len(cfg.Servers))
	}
}

func TestWatcher_InitialCallback(t *testing.T) {
	f := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(f, []byte(yamlConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	called := make(chan *config.File, 1)
	w, err := config.Watch(f, func(c *config.File) { called <- c })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	select {
	case cfg := <-called:
		if len(cfg.Servers) != 2 {
			t.Errorf("initial callback: want 2 servers, got %d", len(cfg.Servers))
		}
	default:
		t.Error("onChange not called immediately")
	}
}

func TestWatcher_Reload(t *testing.T) {
	f := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(f, []byte(yamlConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	calls := make(chan *config.File, 4)
	w, err := config.Watch(f, func(c *config.File) { calls <- c })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	<-calls // initial

	if err := w.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	select {
	case cfg := <-calls:
		if len(cfg.Servers) != 2 {
			t.Errorf("reload: want 2 servers, got %d", len(cfg.Servers))
		}
	default:
		t.Error("onChange not called after Reload")
	}
}

func TestWatcher_Current(t *testing.T) {
	f := filepath.Join(t.TempDir(), "cfg.json")
	if err := os.WriteFile(f, []byte(jsonConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := config.Watch(f, func(_ *config.File) {})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	if c := w.Current(); c == nil {
		t.Error("Current() = nil")
	}
}

func TestWatcher_AutoReload(t *testing.T) {
	f := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(f, []byte(yamlConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	calls := make(chan *config.File, 4)
	w, err := config.Watch(f, func(c *config.File) { calls <- c })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	<-calls // drain initial

	// Ensure mtime differs by sleeping past filesystem resolution.
	time.Sleep(20 * time.Millisecond)

	const updated = `
version: 1
servers:
  - key: a
    stream_id: "0211223344550001"
    transport: udp
    address: "10.0.0.1:5000"
  - key: b
    stream_id: "0211223344550002"
    transport: udp
    address: "10.0.0.2:5000"
  - key: c
    stream_id: "0211223344550003"
    transport: udp
    address: "10.0.0.3:5000"
`
	if err := os.WriteFile(f, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-calls:
		if len(cfg.Servers) != 3 {
			t.Errorf("auto-reload: want 3 servers, got %d", len(cfg.Servers))
		}
	case <-time.After(3 * time.Second):
		t.Error("onChange not called after file was updated")
	}
}
