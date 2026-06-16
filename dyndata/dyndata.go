//fusa:req REQ-DYN-001
//fusa:req REQ-DYN-002
//fusa:req REQ-DYN-003
//fusa:req REQ-DYN-004
//fusa:req REQ-DYN-005
//fusa:req REQ-DYN-006
//fusa:req REQ-DYN-007
//fusa:req REQ-DYN-008

// Package dyndata provides a runtime schema registry and typed payload codec
// for schema-aware command delivery over any rcp.Controller transport.
package dyndata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// FieldKind is the allowed wire type for a schema field.
type FieldKind string

const (
	KindString FieldKind = "string"
	KindInt    FieldKind = "int"
	KindFloat  FieldKind = "float"
	KindBool   FieldKind = "bool"
	KindBytes  FieldKind = "bytes"
)

// Field describes one typed field within a Schema.
type Field struct {
	Name     string
	Kind     FieldKind
	Required bool
}

// Schema describes the typed structure of a command payload.
type Schema struct {
	Name    string
	Version int
	Fields  []Field
}

var (
	ErrAlreadyRegistered = errors.New("rcp/dyndata: schema already registered")
	ErrSchemaNotFound    = errors.New("rcp/dyndata: schema not found")
	ErrFieldTypeMismatch = errors.New("rcp/dyndata: field type mismatch")
	ErrUnknownField      = errors.New("rcp/dyndata: unknown field in payload")
	ErrMissingField      = errors.New("rcp/dyndata: missing required field")
)

// Registry is a thread-safe schema store.
type Registry struct {
	mu      sync.RWMutex
	schemas map[string]Schema
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{schemas: make(map[string]Schema)}
}

// Register stores s in the registry.
// Returns ErrAlreadyRegistered if a schema with the same name already exists.
func (r *Registry) Register(s Schema) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.schemas[s.Name]; ok {
		return fmt.Errorf("%w: %s", ErrAlreadyRegistered, s.Name)
	}
	r.schemas[s.Name] = s
	return nil
}

// Lookup retrieves the schema named name.
// Returns ErrSchemaNotFound if no such schema is registered.
func (r *Registry) Lookup(name string) (Schema, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.schemas[name]
	if !ok {
		return Schema{}, fmt.Errorf("%w: %s", ErrSchemaNotFound, name)
	}
	return s, nil
}

// List returns all registered schemas in unspecified order.
func (r *Registry) List() []Schema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Schema, 0, len(r.schemas))
	for _, s := range r.schemas {
		out = append(out, s)
	}
	return out
}

// Payload is a free-form key-value map that carries typed command data.
type Payload map[string]any

// Encode validates p against s and serialises it to JSON bytes.
// Returns ErrUnknownField if p contains a key not declared in s.
// Returns ErrMissingField if a required field is absent.
// Returns ErrFieldTypeMismatch if a value's Go type is incompatible with the declared FieldKind.
func Encode(s Schema, p Payload) ([]byte, error) {
	fieldMap := make(map[string]Field, len(s.Fields))
	for _, f := range s.Fields {
		fieldMap[f.Name] = f
	}
	for key := range p {
		if _, ok := fieldMap[key]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownField, key)
		}
	}
	for _, f := range s.Fields {
		val, present := p[f.Name]
		if !present {
			if f.Required {
				return nil, fmt.Errorf("%w: %s", ErrMissingField, f.Name)
			}
			continue
		}
		if err := checkKind(f, val); err != nil {
			return nil, err
		}
	}
	return json.Marshal(p)
}

// Decode deserialises JSON bytes into a Payload.
func Decode(_ Schema, b []byte) (Payload, error) {
	var p Payload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("rcp/dyndata: decode: %w", err)
	}
	return p, nil
}

func checkKind(f Field, val any) error {
	var ok bool
	switch f.Kind {
	case KindString:
		_, ok = val.(string)
	case KindInt:
		switch val.(type) {
		case int, int8, int16, int32, int64,
			uint, uint8, uint16, uint32, uint64,
			float64: // JSON numbers unmarshal as float64
			ok = true
		}
	case KindFloat:
		switch val.(type) {
		case float32, float64:
			ok = true
		}
	case KindBool:
		_, ok = val.(bool)
	case KindBytes:
		// bytes are carried as base64 strings in JSON
		_, ok = val.(string)
		if !ok {
			_, ok = val.([]byte)
		}
	default:
		ok = true
	}
	if !ok {
		return fmt.Errorf("%w: field %q expects %s, got %T", ErrFieldTypeMismatch, f.Name, f.Kind, val)
	}
	return nil
}

// TypedController wraps an rcp.Controller with schema-aware payload encoding.
// Send and Subscribe delegate directly to the inner controller.
// Use SendTyped to have the payload encoded against a registered schema.
type TypedController struct {
	inner    rcp.Controller
	registry *Registry
	closed   atomic.Bool
}

// NewTypedController wraps inner with schema-aware encoding using r.
func NewTypedController(inner rcp.Controller, r *Registry) *TypedController {
	return &TypedController{inner: inner, registry: r}
}

// Zone implements rcp.Controller.
func (c *TypedController) Zone() rcp.Zone { return c.inner.Zone() }

// Send implements rcp.Controller — delegates to the inner controller as-is.
func (c *TypedController) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	return c.inner.Send(ctx, cmd)
}

// SendTyped encodes payload using the schema named schemaName, then calls
// inner.Send with cmd.Payload set to the encoded bytes.
func (c *TypedController) SendTyped(ctx context.Context, cmd *rcp.Command, schemaName string, payload Payload) (*rcp.Response, error) {
	s, err := c.registry.Lookup(schemaName)
	if err != nil {
		return nil, err
	}
	encoded, err := Encode(s, payload)
	if err != nil {
		return nil, err
	}
	safe := *cmd
	safe.Payload = encoded
	return c.inner.Send(ctx, &safe)
}

// Subscribe implements rcp.Controller.
func (c *TypedController) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	return c.inner.Subscribe(ctx)
}

// Close implements rcp.Controller — idempotent.
func (c *TypedController) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.inner.Close()
}
