//fusa:test REQ-CG-001
//fusa:test REQ-CG-002
//fusa:test REQ-CG-003
//fusa:test REQ-CG-004
//fusa:test REQ-CG-005
//fusa:test REQ-CG-006
//fusa:test REQ-CG-007
//fusa:test REQ-CG-008

package codegen_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/SoundMatt/go-RCP/codegen"
)

const yamlManifest = `
version: 1
package: myzones
zones:
  - name: front-left
    zone_id: 1
    asil: ASIL-B
    commands:
      - name: set-light
        code: 100
  - name: rear-right
    zone_id: 4
    asil: ASIL-A
    commands: []
`

const jsonManifest = `{
  "version": 1,
  "package": "myzones",
  "zones": [
    {"name": "central", "zone_id": 5, "asil": "ASIL-B", "commands": []}
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
	if m.Package != "myzones" {
		t.Errorf("package = %q, want myzones", m.Package)
	}
	if len(m.Zones) != 2 {
		t.Fatalf("want 2 zones, got %d", len(m.Zones))
	}
	if m.Zones[0].Name != "front-left" {
		t.Errorf("zone[0].Name = %q, want front-left", m.Zones[0].Name)
	}
}

func TestParseManifest_JSON(t *testing.T) {
	m, err := codegen.ParseManifest(strings.NewReader(jsonManifest), ".json")
	if err != nil {
		t.Fatalf("ParseManifest JSON: %v", err)
	}
	if len(m.Zones) != 1 {
		t.Fatalf("want 1 zone, got %d", len(m.Zones))
	}
}

func TestParseManifest_UnsupportedExtension(t *testing.T) {
	_, err := codegen.ParseManifest(strings.NewReader("{}"), ".toml")
	if err == nil {
		t.Error("want error for unsupported extension")
	}
}

func TestParseManifest_InvalidVersion(t *testing.T) {
	const bad = `{"version": 99, "package": "x", "zones": []}`
	_, err := codegen.ParseManifest(strings.NewReader(bad), ".json")
	if !errors.Is(err, codegen.ErrInvalidVersion) {
		t.Errorf("want ErrInvalidVersion, got %v", err)
	}
}

func TestParseManifest_MissingPackage(t *testing.T) {
	const bad = `{"version": 1, "zones": []}`
	_, err := codegen.ParseManifest(strings.NewReader(bad), ".json")
	if !errors.Is(err, codegen.ErrMissingPackage) {
		t.Errorf("want ErrMissingPackage, got %v", err)
	}
}

func TestParseManifest_EmptyZoneName(t *testing.T) {
	const bad = `{"version": 1, "package": "x", "zones": [{"name": "", "zone_id": 1}]}`
	_, err := codegen.ParseManifest(strings.NewReader(bad), ".json")
	if !errors.Is(err, codegen.ErrEmptyName) {
		t.Errorf("want ErrEmptyName, got %v", err)
	}
}

func TestGenerate_ProducesImplAndTest(t *testing.T) {
	m, err := codegen.ParseManifest(strings.NewReader(yamlManifest), ".yaml")
	if err != nil {
		t.Fatal(err)
	}
	files, err := codegen.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Expect 2 zones × 2 files (impl + test) = 4
	if len(files) != 4 {
		t.Fatalf("want 4 files, got %d", len(files))
	}
	// Each impl file should compile (have valid Go source with package declaration).
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
	if !strings.Contains(content, "//fusa:req REQ-C-001") {
		t.Errorf("impl missing //fusa:req: %s", content[:min(200, len(content))])
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
	if !strings.Contains(content, "//fusa:test REQ-C-001") {
		t.Errorf("test file missing //fusa:test: %s", content[:min(200, len(content))])
	}
}

func TestGenerate_CommandConstInImpl(t *testing.T) {
	m, err := codegen.ParseManifest(strings.NewReader(yamlManifest), ".yaml")
	if err != nil {
		t.Fatal(err)
	}
	files, err := codegen.Generate(m)
	if err != nil {
		t.Fatal(err)
	}
	// front-left has set-light command
	implContent := string(files[0].Content)
	if !strings.Contains(implContent, "CmdSetLight") {
		t.Errorf("impl missing CmdSetLight constant: %s", implContent[:min(300, len(implContent))])
	}
}

func TestGenerateRequirements_ProducesEightPerZone(t *testing.T) {
	m, err := codegen.ParseManifest(strings.NewReader(yamlManifest), ".yaml")
	if err != nil {
		t.Fatal(err)
	}
	reqs := codegen.GenerateRequirements(m)
	// 2 zones × 8 reqs = 16
	if len(reqs) != 16 {
		t.Errorf("want 16 reqs, got %d", len(reqs))
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
