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

// EndpointSpec is the manifest entry for one declared endpoint on a server.
// See doc.go's note on Type for why it is a free-text label rather than a
// regmap.EndpointType.
type EndpointSpec struct {
	Name string `yaml:"name"        json:"name"`
	Addr uint8  `yaml:"byte_bus_id" json:"byte_bus_id"`
	Type string `yaml:"type"        json:"type"`
	ASIL ASIL   `yaml:"asil"        json:"asil"`
}

// ServerSpec is the manifest entry for one declared RC Server.
type ServerSpec struct {
	Name      string         `yaml:"name"      json:"name"`
	StreamID  string         `yaml:"stream_id" json:"stream_id"` // 16 hex chars (8 bytes), same encoding as config.ServerEntry
	Endpoints []EndpointSpec `yaml:"endpoints" json:"endpoints"`
}

// Manifest is the top-level manifest file structure.
type Manifest struct {
	Version int          `yaml:"version" json:"version"`
	Package string       `yaml:"package" json:"package"`
	Servers []ServerSpec `yaml:"servers" json:"servers"`
}

var (
	ErrInvalidVersion = errors.New("rcp/codegen: unsupported manifest version")
	ErrMissingPackage = errors.New("rcp/codegen: manifest missing package field")
	ErrEmptyName      = errors.New("rcp/codegen: server and endpoint names must not be empty")
	ErrReservedAddr   = errors.New("rcp/codegen: byte_bus_id 0 is reserved for EP0")
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
	for _, s := range m.Servers {
		if s.Name == "" {
			return nil, ErrEmptyName
		}
		for _, ep := range s.Endpoints {
			if ep.Name == "" {
				return nil, ErrEmptyName
			}
			if ep.Addr == 0 {
				return nil, ErrReservedAddr
			}
		}
	}
	return &m, nil
}

// GeneratedFile is one generated Go source file.
type GeneratedFile struct {
	Name    string // suggested filename, e.g. "frontleft_gpio_endpoint.go"
	Content []byte // gofmt-formatted Go source
}

// Generate produces Go source files from manifest m: one stub impl file and
// one test-skeleton file per declared endpoint, across every declared
// server.
func Generate(m *Manifest) ([]GeneratedFile, error) {
	var files []GeneratedFile
	for _, s := range m.Servers {
		for _, ep := range s.Endpoints {
			impl, err := generateImpl(m.Package, s, ep)
			if err != nil {
				return nil, fmt.Errorf("rcp/codegen: server %q endpoint %q impl: %w", s.Name, ep.Name, err)
			}
			files = append(files, impl)

			test, err := generateTest(m.Package, s, ep)
			if err != nil {
				return nil, fmt.Errorf("rcp/codegen: server %q endpoint %q test: %w", s.Name, ep.Name, err)
			}
			files = append(files, test)
		}
	}
	return files, nil
}

// GenerateRequirements produces .fusa-reqs.json entries for every declared
// endpoint across every declared server in m.
func GenerateRequirements(m *Manifest) []map[string]string {
	var reqs []map[string]string
	for _, s := range m.Servers {
		for _, ep := range s.Endpoints {
			prefix := reqPrefix(s.Name, ep.Name)
			for i, desc := range reqDescriptions(s, ep) {
				reqs = append(reqs, map[string]string{
					"id":       fmt.Sprintf("%s-%03d", prefix, i+1),
					"title":    desc.title,
					"text":     desc.text,
					"standard": "iso26262",
					"level":    string(ep.ASIL),
					"asil":     string(ep.ASIL),
				})
			}
		}
	}
	return reqs
}

type reqDesc struct{ title, text string }

func reqDescriptions(s ServerSpec, ep EndpointSpec) []reqDesc {
	typeName := goTypeName(s.Name) + goTypeName(ep.Name)
	return []reqDesc{
		{fmt.Sprintf("%s HandleRequest rejects wrong endpoint", typeName), fmt.Sprintf("The %s endpoint stub shall reject a request whose ByteBusID is not %d.", typeName, ep.Addr)},
		{fmt.Sprintf("%s HandleRequest answers a read request", typeName), fmt.Sprintf("The %s endpoint stub shall answer a FlagRead request with FlagResponse set and FlagRead preserved.", typeName)},
		{fmt.Sprintf("%s HandleRequest answers a write request", typeName), fmt.Sprintf("The %s endpoint stub shall answer a FlagWrite request with FlagResponse set and FlagWrite preserved.", typeName)},
		{fmt.Sprintf("%s HandleRequest is race-free", typeName), fmt.Sprintf("The %s endpoint stub shall be safe for concurrent HandleRequest calls without data races.", typeName)},
		{fmt.Sprintf("%s Close idempotent", typeName), fmt.Sprintf("The %s endpoint stub Close shall be safe to call multiple times.", typeName)},
		{fmt.Sprintf("%s HandleRequest after Close returns ErrClosed", typeName), fmt.Sprintf("The %s endpoint stub shall return ErrClosed for HandleRequest after Close.", typeName)},
	}
}

var implTmpl = template.Must(template.New("impl").Parse(`// Code generated by rcp/codegen. DO NOT EDIT.
{{- range .FusaReqs}}
//fusa:req {{.}}
{{- end}}

package {{.Package}}

import (
	"fmt"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
)

// Addr{{.TypeName}} is the declared byte_bus_id for the {{.EndpointType}} endpoint "{{.EndpointName}}" on server "{{.ServerName}}".
const Addr{{.TypeName}} avtp.ByteBusID = {{.Addr}}

// {{.TypeName}}Endpoint is a generated request.Handler stub for the
// {{.EndpointType}} endpoint "{{.EndpointName}}" on server "{{.ServerName}}"
// (ASIL {{.ASIL}}). Replace HandleRequest's body with real endpoint logic.
type {{.TypeName}}Endpoint struct {
	closed atomic.Bool
}

// New{{.TypeName}}Endpoint returns a new {{.TypeName}}Endpoint.
func New{{.TypeName}}Endpoint() *{{.TypeName}}Endpoint {
	return &{{.TypeName}}Endpoint{}
}

// HandleRequest implements request.Handler for Addr{{.TypeName}}.
func (e *{{.TypeName}}Endpoint) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	if e.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/{{.Package}}: %s: closed", "{{.EndpointName}}")
	}
	if req.ByteBusID != Addr{{.TypeName}} {
		return acf.Message{}, fmt.Errorf("rcp/{{.Package}}: %s: wrong endpoint", "{{.EndpointName}}")
	}
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
	}, nil
}

// Close marks the endpoint closed. Safe to call multiple times.
func (e *{{.TypeName}}Endpoint) Close() error {
	e.closed.Store(true)
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
	Package      string
	TypeName     string
	ServerName   string
	EndpointName string
	EndpointType string
	Addr         uint8
	ASIL         ASIL
	FusaReqs     []string
}

type testData struct {
	Package   string
	TypeName  string
	FusaTests []string
}

func generateImpl(pkg string, s ServerSpec, ep EndpointSpec) (GeneratedFile, error) {
	prefix := reqPrefix(s.Name, ep.Name)
	var reqs []string
	for i := 1; i <= 6; i++ {
		reqs = append(reqs, fmt.Sprintf("%s-%03d", prefix, i))
	}
	typeName := goTypeName(s.Name) + goTypeName(ep.Name)
	d := implData{
		Package:      pkg,
		TypeName:     typeName,
		ServerName:   s.Name,
		EndpointName: ep.Name,
		EndpointType: ep.Type,
		Addr:         ep.Addr,
		ASIL:         ep.ASIL,
		FusaReqs:     reqs,
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
		Name:    strings.ToLower(s.Name) + "_" + strings.ToLower(ep.Name) + "_endpoint.go",
		Content: formatted,
	}, nil
}

func generateTest(pkg string, s ServerSpec, ep EndpointSpec) (GeneratedFile, error) {
	prefix := reqPrefix(s.Name, ep.Name)
	var tests []string
	for i := 1; i <= 6; i++ {
		tests = append(tests, fmt.Sprintf("%s-%03d", prefix, i))
	}
	typeName := goTypeName(s.Name) + goTypeName(ep.Name)
	d := testData{
		Package:   pkg,
		TypeName:  typeName,
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
		Name:    strings.ToLower(s.Name) + "_" + strings.ToLower(ep.Name) + "_endpoint_test.go",
		Content: formatted,
	}, nil
}

// goTypeName converts a name like "front-left" to "FrontLeft".
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

// reqPrefix returns REQ-XXYY from a server/endpoint name pair (e.g.
// "front-left"/"main-gpio" -> "REQ-FLMG").
func reqPrefix(serverName, endpointName string) string {
	var b strings.Builder
	b.WriteString("REQ-")
	for _, name := range []string{serverName, endpointName} {
		parts := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
		for _, p := range parts {
			if len(p) > 0 {
				b.WriteRune(unicode.ToUpper(rune(p[0])))
			}
		}
	}
	return b.String()
}
