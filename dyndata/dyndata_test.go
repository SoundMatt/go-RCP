//fusa:test REQ-DYN-001
//fusa:test REQ-DYN-002
//fusa:test REQ-DYN-003
//fusa:test REQ-DYN-004
//fusa:test REQ-DYN-005
//fusa:test REQ-DYN-006
//fusa:test REQ-DYN-007
//fusa:test REQ-DYN-008

package dyndata_test

import (
	"context"
	"errors"
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/dyndata"
	"github.com/SoundMatt/go-RCP/mock"
)

var eventSchema = dyndata.Schema{
	Name:    "motion_event",
	Version: 1,
	Fields: []dyndata.Field{
		{Name: "zone_id", Kind: dyndata.KindString, Required: true},
		{Name: "speed_ms", Kind: dyndata.KindFloat, Required: true},
		{Name: "triggered", Kind: dyndata.KindBool, Required: false},
	},
}

// REQ-DYN-001: Register stores a schema; Lookup retrieves it.
func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := dyndata.NewRegistry()
	if err := r.Register(eventSchema); err != nil {
		t.Fatalf("Register: %v", err)
	}
	s, err := r.Lookup(eventSchema.Name)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if s.Name != eventSchema.Name {
		t.Errorf("Lookup name = %q, want %q", s.Name, eventSchema.Name)
	}
}

// REQ-DYN-002: Register returns ErrAlreadyRegistered on duplicate name.
func TestRegistry_DuplicateRegister(t *testing.T) {
	r := dyndata.NewRegistry()
	_ = r.Register(eventSchema)
	err := r.Register(eventSchema)
	if !errors.Is(err, dyndata.ErrAlreadyRegistered) {
		t.Errorf("want ErrAlreadyRegistered, got %v", err)
	}
}

// REQ-DYN-003: Lookup returns ErrSchemaNotFound for unknown name.
func TestRegistry_LookupMiss(t *testing.T) {
	r := dyndata.NewRegistry()
	_, err := r.Lookup("nonexistent")
	if !errors.Is(err, dyndata.ErrSchemaNotFound) {
		t.Errorf("want ErrSchemaNotFound, got %v", err)
	}
}

// REQ-DYN-004: Encode marshals a valid payload to JSON bytes.
func TestEncode_Valid(t *testing.T) {
	p := dyndata.Payload{
		"zone_id":   "front-left",
		"speed_ms":  12.5,
		"triggered": true,
	}
	b, err := dyndata.Encode(eventSchema, p)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(b) == 0 {
		t.Error("Encode returned empty bytes")
	}
}

// REQ-DYN-005: Decode unmarshals bytes back to a Payload map.
func TestDecode_RoundTrip(t *testing.T) {
	p := dyndata.Payload{"zone_id": "rear-right", "speed_ms": 3.14}
	b, err := dyndata.Encode(eventSchema, p)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := dyndata.Decode(eventSchema, b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got["zone_id"] != "rear-right" {
		t.Errorf("zone_id = %v, want rear-right", got["zone_id"])
	}
}

// REQ-DYN-006: Encode returns ErrFieldTypeMismatch for wrong field type.
func TestEncode_TypeMismatch(t *testing.T) {
	p := dyndata.Payload{
		"zone_id":  42, // int, not string
		"speed_ms": 1.0,
	}
	_, err := dyndata.Encode(eventSchema, p)
	if !errors.Is(err, dyndata.ErrFieldTypeMismatch) {
		t.Errorf("want ErrFieldTypeMismatch, got %v", err)
	}
}

// REQ-DYN-007: Encode returns ErrUnknownField for undeclared payload keys.
func TestEncode_UnknownField(t *testing.T) {
	p := dyndata.Payload{
		"zone_id":  "fl",
		"speed_ms": 1.0,
		"extra":    "unexpected",
	}
	_, err := dyndata.Encode(eventSchema, p)
	if !errors.Is(err, dyndata.ErrUnknownField) {
		t.Errorf("want ErrUnknownField, got %v", err)
	}
}

// REQ-DYN-008: TypedController.SendTyped encodes payload and delegates to inner.Send.
func TestTypedController_SendTyped(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		if len(cmd.Payload) == 0 {
			t.Error("SendTyped sent empty payload")
		}
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	reg := dyndata.NewRegistry()
	if err := reg.Register(eventSchema); err != nil {
		t.Fatal(err)
	}
	tc := dyndata.NewTypedController(inner, reg)
	defer func() { _ = tc.Close() }()

	cmd := &rcp.Command{
		Zone:     rcp.ZoneFrontLeft,
		Type:     rcp.CmdSet,
		Priority: rcp.PriorityNormal,
	}
	p := dyndata.Payload{"zone_id": "front-left", "speed_ms": 7.2}
	resp, err := tc.SendTyped(context.Background(), cmd, eventSchema.Name, p)
	if err != nil {
		t.Fatalf("SendTyped: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("response status = %v, want StatusOK", resp.Status)
	}
}

func TestTypedController_SendTyped_SchemaNotFound(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	tc := dyndata.NewTypedController(inner, dyndata.NewRegistry())
	defer func() { _ = tc.Close() }()

	_, err := tc.SendTyped(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft}, "missing", nil)
	if !errors.Is(err, dyndata.ErrSchemaNotFound) {
		t.Errorf("want ErrSchemaNotFound, got %v", err)
	}
}

func TestTypedController_ClosedIdempotent(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	tc := dyndata.NewTypedController(inner, dyndata.NewRegistry())
	if err := tc.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := tc.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestRegistry_List(t *testing.T) {
	r := dyndata.NewRegistry()
	_ = r.Register(eventSchema)
	_ = r.Register(dyndata.Schema{Name: "other", Version: 1})
	if n := len(r.List()); n != 2 {
		t.Errorf("List len = %d, want 2", n)
	}
}
