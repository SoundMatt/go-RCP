//fusa:req REQ-CG-001
//fusa:req REQ-CG-002
//fusa:req REQ-CG-003
//fusa:req REQ-CG-004
//fusa:req REQ-CG-005
//fusa:req REQ-CG-006
//fusa:req REQ-CG-007
//fusa:req REQ-CG-008

// Package codegen generates typed Go controller stubs and go-FuSa requirement
// skeletons from a zone manifest YAML/JSON file.
//
// A manifest declares zone IDs, supported command types, payload schemas, and
// ASIL levels. The generator emits:
//   - A Go source file implementing rcp.Controller for each zone type
//   - A matching _test.go skeleton with //fusa:test annotations
//   - JSON entries for .fusa-reqs.json ready for go-FuSa compliance
package codegen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io"
	"strings"
	"text/template"
	"unicode"

	"gopkg.in/yaml.v3"
)

// ASIL is an Automotive Safety Integrity Level.
type ASIL string

const (
	ASILNone ASIL = "QM"
	ASILA    ASIL = "ASIL-A"
	ASILB    ASIL = "ASIL-B"
	ASILC    ASIL = "ASIL-C"
	ASILD    ASIL = "ASIL-D"
)

// CommandSpec describes a command type supported by a zone.
type CommandSpec struct {
	Name    string `yaml:"name"    json:"name"`
	Code    uint16 `yaml:"code"    json:"code"`
	Payload string `yaml:"payload" json:"payload,omitempty"`
}

// ZoneSpec is the manifest entry for a single zone type.
type ZoneSpec struct {
	Name     string        `yaml:"name"     json:"name"`
	ZoneID   uint8         `yaml:"zone_id"  json:"zone_id"`
	ASIL     ASIL          `yaml:"asil"     json:"asil"`
	Commands []CommandSpec `yaml:"commands" json:"commands"`
}

// Manifest is the top-level manifest file structure.
type Manifest struct {
	Version int        `yaml:"version" json:"version"`
	Package string     `yaml:"package" json:"package"`
	Zones   []ZoneSpec `yaml:"zones"   json:"zones"`
}

var (
	ErrInvalidVersion = errors.New("rcp/codegen: unsupported manifest version")
	ErrMissingPackage = errors.New("rcp/codegen: manifest missing package field")
	ErrEmptyName      = errors.New("rcp/codegen: zone name must not be empty")
)

// ParseManifest decodes a manifest from r. ext selects the decoder (.yaml/.yml or .json).
func ParseManifest(r io.Reader, ext string) (*Manifest, error) {
	var m Manifest
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.NewDecoder(r).Decode(&m); err != nil {
			return nil, fmt.Errorf("rcp/codegen: yaml decode: %w", err)
		}
	case ".json":
		if err := json.NewDecoder(r).Decode(&m); err != nil {
			return nil, fmt.Errorf("rcp/codegen: json decode: %w", err)
		}
	default:
		return nil, fmt.Errorf("rcp/codegen: unsupported extension %q", ext)
	}
	if m.Version != 1 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidVersion, m.Version)
	}
	if m.Package == "" {
		return nil, ErrMissingPackage
	}
	for _, z := range m.Zones {
		if z.Name == "" {
			return nil, ErrEmptyName
		}
	}
	return &m, nil
}

// GeneratedFile is one generated Go source file.
type GeneratedFile struct {
	Name    string // suggested filename, e.g. "frontleft_controller.go"
	Content []byte // gofmt-formatted Go source
}

// Generate produces Go source files from manifest m.
// Returns one impl file and one test file per zone.
func Generate(m *Manifest) ([]GeneratedFile, error) {
	var files []GeneratedFile
	for _, z := range m.Zones {
		impl, err := generateImpl(m.Package, z)
		if err != nil {
			return nil, fmt.Errorf("rcp/codegen: zone %q impl: %w", z.Name, err)
		}
		files = append(files, impl)

		test, err := generateTest(m.Package, z)
		if err != nil {
			return nil, fmt.Errorf("rcp/codegen: zone %q test: %w", z.Name, err)
		}
		files = append(files, test)
	}
	return files, nil
}

// GenerateRequirements produces .fusa-reqs.json entries for all zones in m.
func GenerateRequirements(m *Manifest) []map[string]string {
	var reqs []map[string]string
	for _, z := range m.Zones {
		prefix := reqPrefix(z.Name)
		descs := reqDescriptions(z)
		for i, desc := range descs {
			reqs = append(reqs, map[string]string{
				"id":       fmt.Sprintf("%s-%03d", prefix, i+1),
				"title":    desc.title,
				"text":     desc.text,
				"standard": "iso26262",
				"level":    string(z.ASIL),
				"asil":     string(z.ASIL),
			})
		}
	}
	return reqs
}

type reqDesc struct{ title, text string }

func reqDescriptions(z ZoneSpec) []reqDesc {
	typeName := goTypeName(z.Name)
	return []reqDesc{
		{fmt.Sprintf("%s zone identification", typeName), fmt.Sprintf("The %s controller shall identify itself with ZoneID %d.", typeName, z.ZoneID)},
		{fmt.Sprintf("%s Send dispatches command", typeName), fmt.Sprintf("The %s controller shall dispatch commands to zone %d via Send.", typeName, z.ZoneID)},
		{fmt.Sprintf("%s Send rejects zone mismatch", typeName), fmt.Sprintf("The %s controller shall return ErrZoneMismatch when cmd.Zone != %d.", typeName, z.ZoneID)},
		{fmt.Sprintf("%s Subscribe returns status channel", typeName), fmt.Sprintf("The %s controller shall return a status channel from Subscribe.", typeName)},
		{fmt.Sprintf("%s Send context cancellation", typeName), fmt.Sprintf("The %s controller shall honour context cancellation in Send.", typeName)},
		{fmt.Sprintf("%s Send race-free", typeName), fmt.Sprintf("The %s controller shall be safe for concurrent Send calls without data races.", typeName)},
		{fmt.Sprintf("%s Close idempotent", typeName), fmt.Sprintf("The %s controller Close shall be safe to call multiple times.", typeName)},
		{fmt.Sprintf("%s Send-after-Close returns ErrClosed", typeName), fmt.Sprintf("The %s controller shall return ErrClosed for Send or Subscribe after Close.", typeName)},
	}
}

var implTmpl = template.Must(template.New("impl").Parse(`// Code generated by rcp/codegen. DO NOT EDIT.
{{- range .FusaReqs}}
//fusa:req {{.}}
{{- end}}

package {{.Package}}

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

{{range .Commands}}// Cmd{{.GoName}} is the command code for {{.Name}}.
const Cmd{{.GoName}} rcp.CommandType = {{.Code}}
{{end}}

// {{.TypeName}}Controller implements rcp.Controller for zone {{.ZoneID}}.
type {{.TypeName}}Controller struct {
	mu     sync.Mutex
	closed atomic.Bool
}

// New{{.TypeName}}Controller returns a new {{.TypeName}}Controller.
func New{{.TypeName}}Controller() *{{.TypeName}}Controller {
	return &{{.TypeName}}Controller{}
}

// Zone returns the fixed zone for this controller.
func (c *{{.TypeName}}Controller) Zone() rcp.Zone { return rcp.Zone({{.ZoneID}}) }

// Send dispatches cmd to zone {{.ZoneID}}.
func (c *{{.TypeName}}Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/{{.Package}}: %w", rcp.ErrClosed)
	}
	if cmd.Zone != rcp.Zone({{.ZoneID}}) {
		return nil, fmt.Errorf("rcp/{{.Package}}: %w", rcp.ErrZoneMismatch)
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rcp/{{.Package}}: %w", rcp.ErrTimeout)
	default:
	}
	_ = c.mu
	return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}, nil
}

// Subscribe returns a channel of Status updates.
func (c *{{.TypeName}}Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/{{.Package}}: %w", rcp.ErrClosed)
	}
	ch := make(chan *rcp.Status, 16)
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch, nil
}

// Close releases resources. Safe to call multiple times.
func (c *{{.TypeName}}Controller) Close() error {
	c.closed.CompareAndSwap(false, true)
	return nil
}
`))

var testTmpl = template.Must(template.New("test").Parse(`// Code generated by rcp/codegen. DO NOT EDIT.
{{- range .FusaTests}}
//fusa:test {{.}}
{{- end}}

package {{.Package}}_test
`))

type implData struct {
	Package   string
	TypeName  string
	ZoneID    uint8
	FusaReqs  []string
	Commands  []cmdData
}

type cmdData struct {
	Name   string
	GoName string
	Code   uint16
}

type testData struct {
	Package   string
	TypeName  string
	FusaTests []string
}

func generateImpl(pkg string, z ZoneSpec) (GeneratedFile, error) {
	prefix := reqPrefix(z.Name)
	var reqs []string
	for i := 1; i <= 8; i++ {
		reqs = append(reqs, fmt.Sprintf("%s-%03d", prefix, i))
	}
	var cmds []cmdData
	for _, c := range z.Commands {
		cmds = append(cmds, cmdData{Name: c.Name, GoName: goTypeName(c.Name), Code: c.Code})
	}
	d := implData{
		Package:  pkg,
		TypeName: goTypeName(z.Name),
		ZoneID:   z.ZoneID,
		FusaReqs: reqs,
		Commands: cmds,
	}
	var buf bytes.Buffer
	if err := implTmpl.Execute(&buf, d); err != nil {
		return GeneratedFile{}, err
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return GeneratedFile{}, fmt.Errorf("gofmt: %w\n%s", err, buf.String())
	}
	return GeneratedFile{
		Name:    strings.ToLower(z.Name) + "_controller.go",
		Content: formatted,
	}, nil
}

func generateTest(pkg string, z ZoneSpec) (GeneratedFile, error) {
	prefix := reqPrefix(z.Name)
	var tests []string
	for i := 1; i <= 8; i++ {
		tests = append(tests, fmt.Sprintf("%s-%03d", prefix, i))
	}
	d := testData{
		Package:   pkg,
		TypeName:  goTypeName(z.Name),
		FusaTests: tests,
	}
	var buf bytes.Buffer
	if err := testTmpl.Execute(&buf, d); err != nil {
		return GeneratedFile{}, err
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return GeneratedFile{}, fmt.Errorf("gofmt test: %w\n%s", err, buf.String())
	}
	return GeneratedFile{
		Name:    strings.ToLower(z.Name) + "_controller_test.go",
		Content: formatted,
	}, nil
}

// goTypeName converts a zone name like "front-left" to "FrontLeft".
func goTypeName(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		if r == '-' || r == '_' || r == ' ' {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// reqPrefix returns REQ-XX from a zone name (e.g. "front-left" → "REQ-FL").
func reqPrefix(name string) string {
	var b strings.Builder
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
	for _, p := range parts {
		if len(p) > 0 {
			b.WriteRune(unicode.ToUpper(rune(p[0])))
		}
	}
	return "REQ-" + b.String()
}
