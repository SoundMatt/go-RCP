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

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/config"
)

const yamlConfig = `
version: 1
zones:
  - zone: 1
    transport: udp
    address: "192.168.1.10:5000"
  - zone: 2
    transport: tls
    address: "192.168.1.11:5001"
    cert_file: "/etc/rcp/zone2.crt"
    key_file:  "/etc/rcp/zone2.key"
    ca_file:   "/etc/rcp/ca.crt"
`

const jsonConfig = `{
  "version": 1,
  "zones": [
    {"zone": 1, "transport": "udp", "address": "10.0.0.1:5000"},
    {"zone": 3, "transport": "mock", "address": ""}
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
	if len(cfg.Zones) != 2 {
		t.Fatalf("want 2 zones, got %d", len(cfg.Zones))
	}
	if cfg.Zones[0].Zone != rcp.ZoneFrontLeft {
		t.Errorf("zone[0] = %v, want front-left", cfg.Zones[0].Zone)
	}
	if cfg.Zones[0].Transport != config.TransportUDP {
		t.Errorf("transport = %q, want udp", cfg.Zones[0].Transport)
	}
}

func TestDecode_JSON(t *testing.T) {
	cfg, err := config.Decode(strings.NewReader(jsonConfig), ".json")
	if err != nil {
		t.Fatalf("Decode JSON: %v", err)
	}
	if len(cfg.Zones) != 2 {
		t.Fatalf("want 2 zones, got %d", len(cfg.Zones))
	}
}

func TestDecode_YAML_Alias(t *testing.T) {
	cfg, err := config.Decode(strings.NewReader(yamlConfig), ".yml")
	if err != nil {
		t.Fatalf("Decode .yml: %v", err)
	}
	if len(cfg.Zones) != 2 {
		t.Errorf("want 2 zones, got %d", len(cfg.Zones))
	}
}

func TestDecode_UnsupportedExtension(t *testing.T) {
	_, err := config.Decode(strings.NewReader("{}"), ".toml")
	if err == nil {
		t.Error("want error for unsupported extension")
	}
}

func TestDecode_InvalidVersion(t *testing.T) {
	const bad = `{"version": 99, "zones": []}`
	_, err := config.Decode(strings.NewReader(bad), ".json")
	if !errors.Is(err, config.ErrInvalidVersion) {
		t.Errorf("want ErrInvalidVersion, got %v", err)
	}
}

func TestDecode_DuplicateZone(t *testing.T) {
	const dup = `{"version": 1, "zones": [
		{"zone": 1, "transport": "mock"},
		{"zone": 1, "transport": "udp"}
	]}`
	_, err := config.Decode(strings.NewReader(dup), ".json")
	if !errors.Is(err, config.ErrDuplicateZone) {
		t.Errorf("want ErrDuplicateZone, got %v", err)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("want error for missing file")
	}
}

func TestLoad_ValidFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "registry.yaml")
	if err := os.WriteFile(f, []byte(yamlConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Zones) != 2 {
		t.Errorf("want 2 zones, got %d", len(cfg.Zones))
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
		if len(cfg.Zones) != 2 {
			t.Errorf("initial callback: want 2 zones, got %d", len(cfg.Zones))
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
		if len(cfg.Zones) != 2 {
			t.Errorf("reload: want 2 zones, got %d", len(cfg.Zones))
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
zones:
  - zone: 1
    transport: udp
    address: "10.0.0.1:5000"
  - zone: 2
    transport: udp
    address: "10.0.0.2:5000"
  - zone: 3
    transport: udp
    address: "10.0.0.3:5000"
`
	if err := os.WriteFile(f, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-calls:
		if len(cfg.Zones) != 3 {
			t.Errorf("auto-reload: want 3 zones, got %d", len(cfg.Zones))
		}
	case <-time.After(3 * time.Second):
		t.Error("onChange not called after file was updated")
	}
}
