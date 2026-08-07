//fusa:test REQ-CG-001
//fusa:test REQ-CG-002
//fusa:test REQ-CG-003
//fusa:test REQ-CG-004
//fusa:test REQ-CG-005
//fusa:test REQ-CG-006
//fusa:test REQ-CG-007
//fusa:test REQ-CG-008
//fusa:test REQ-CG-009

package codegen_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/codegen"
)

const yamlManifest = `
version: 1
package: myservers
servers:
  - name: front-left
    stream_id: "0211223344550001"
    endpoints:
      - name: main-gpio
        byte_bus_id: 1
        type: gpio
        asil: ASIL-B
  - name: rear-right
    stream_id: "02AABBCCDDEE0001"
    endpoints: []
`

const jsonManifest = `{
  "version": 1,
  "package": "myservers",
  "servers": [
    {"name": "central", "stream_id": "02AABBCCDDEE0002", "endpoints": [
      {"name": "adc-a", "byte_bus_id": 5, "type": "adc", "asil": "ASIL-B"}
    ]}
  ]
}`

func TestParseManifest_YAML(t *testing.T) {
	m, err := codegen.ParseManifest(strings.NewReader(yamlManifest), ".yaml")
	if err != nil {
		t.Fatalf("ParseManifest YAML: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("version = %d, want 1", m.Version)
	}
	if m.Package != "myservers" {
		t.Errorf("package = %q, want myservers", m.Package)
	}
	if len(m.Servers) != 2 {
		t.Fatalf("want 2 servers, got %d", len(m.Servers))
	}
	if m.Servers[0].Name != "front-left" {
		t.Errorf("servers[0].Name = %q, want front-left", m.Servers[0].Name)
	}
	if len(m.Servers[0].Endpoints) != 1 || m.Servers[0].Endpoints[0].Name != "main-gpio" {
		t.Errorf("servers[0].Endpoints unexpected: %+v", m.Servers[0].Endpoints)
	}
}

func TestParseManifest_JSON(t *testing.T) {
	m, err := codegen.ParseManifest(strings.NewReader(jsonManifest), ".json")
	if err != nil {
		t.Fatalf("ParseManifest JSON: %v", err)
	}
	if len(m.Servers) != 1 || len(m.Servers[0].Endpoints) != 1 {
		t.Fatalf("unexpected manifest shape: %+v", m)
	}
}

func TestParseManifest_UnsupportedExtension(t *testing.T) {
	_, err := codegen.ParseManifest(strings.NewReader("{}"), ".toml")
	if err == nil {
		t.Error("want error for unsupported extension")
	}
}

func TestParseManifest_InvalidVersion(t *testing.T) {
	const bad = `{"version": 99, "package": "x", "servers": []}`
	_, err := codegen.ParseManifest(strings.NewReader(bad), ".json")
	if !errors.Is(err, codegen.ErrInvalidVersion) {
		t.Errorf("want ErrInvalidVersion, got %v", err)
	}
}

func TestParseManifest_MissingPackage(t *testing.T) {
	const bad = `{"version": 1, "servers": []}`
	_, err := codegen.ParseManifest(strings.NewReader(bad), ".json")
	if !errors.Is(err, codegen.ErrMissingPackage) {
		t.Errorf("want ErrMissingPackage, got %v", err)
	}
}

func TestParseManifest_EmptyEndpointName(t *testing.T) {
	const bad = `{"version": 1, "package": "x", "servers": [{"name": "s", "stream_id": "00", "endpoints": [{"name": "", "byte_bus_id": 1}]}]}`
	_, err := codegen.ParseManifest(strings.NewReader(bad), ".json")
	if !errors.Is(err, codegen.ErrEmptyName) {
		t.Errorf("want ErrEmptyName, got %v", err)
	}
}

func TestParseManifest_ReservedEndpointAddr(t *testing.T) {
	const bad = `{"version": 1, "package": "x", "servers": [{"name": "s", "stream_id": "00", "endpoints": [{"name": "ep0-alias", "byte_bus_id": 0}]}]}`
	_, err := codegen.ParseManifest(strings.NewReader(bad), ".json")
	if !errors.Is(err, codegen.ErrReservedAddr) {
		t.Errorf("want ErrReservedAddr, got %v", err)
	}
}

func TestGenerate_ProducesImplAndTestPerEndpoint(t *testing.T) {
	m, err := codegen.ParseManifest(strings.NewReader(yamlManifest), ".yaml")
	if err != nil {
		t.Fatal(err)
	}
	files, err := codegen.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// 1 endpoint total (front-left/main-gpio; rear-right has none) × 2 files.
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d", len(files))
	}
	for _, f := range files {
		if !strings.Contains(string(f.Content), "package ") {
			t.Errorf("file %q missing package declaration", f.Name)
		}
	}
}

func TestGenerate_ImplContainsFusaReqs(t *testing.T) {
	m, err := codegen.ParseManifest(strings.NewReader(jsonManifest), ".json")
	if err != nil {
		t.Fatal(err)
	}
	files, err := codegen.Generate(m)
	if err != nil {
		t.Fatal(err)
	}
	implFile := files[0]
	content := string(implFile.Content)
	if !strings.Contains(content, "//fusa:req REQ-CAA-001") {
		t.Errorf("impl missing //fusa:req: %s", content[:min(300, len(content))])
	}
}

func TestGenerate_TestContainsFusaTests(t *testing.T) {
	m, err := codegen.ParseManifest(strings.NewReader(jsonManifest), ".json")
	if err != nil {
		t.Fatal(err)
	}
	files, err := codegen.Generate(m)
	if err != nil {
		t.Fatal(err)
	}
	testFile := files[1]
	content := string(testFile.Content)
	if !strings.Contains(content, "//fusa:test REQ-CAA-001") {
		t.Errorf("test file missing //fusa:test: %s", content[:min(300, len(content))])
	}
}

func TestGenerate_ByteBusIDConstInImpl(t *testing.T) {
	m, err := codegen.ParseManifest(strings.NewReader(yamlManifest), ".yaml")
	if err != nil {
		t.Fatal(err)
	}
	files, err := codegen.Generate(m)
	if err != nil {
		t.Fatal(err)
	}
	implContent := string(files[0].Content)
	if !strings.Contains(implContent, "AddrFrontLeftMainGpio") {
		t.Errorf("impl missing byte_bus_id const: %s", implContent[:min(400, len(implContent))])
	}
}

func TestGenerateRequirements_ProducesSixPerEndpoint(t *testing.T) {
	m, err := codegen.ParseManifest(strings.NewReader(yamlManifest), ".yaml")
	if err != nil {
		t.Fatal(err)
	}
	reqs := codegen.GenerateRequirements(m)
	// 1 endpoint total × 6 reqs = 6.
	if len(reqs) != 6 {
		t.Errorf("want 6 reqs, got %d", len(reqs))
	}
	for _, r := range reqs {
		if r["asil"] == "" {
			t.Errorf("req %q missing asil field", r["id"])
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
