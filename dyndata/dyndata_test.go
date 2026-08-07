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
	"sync"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/dyndata"
	"github.com/SoundMatt/go-RCP/v9/request"
	"github.com/SoundMatt/go-RCP/v9/server"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

const testEndpoint = avtp.ByteBusID(1)

type recordingHandler struct {
	mu       sync.Mutex
	lastBody []byte
}

func (h *recordingHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	h.mu.Lock()
	h.lastBody = req.Body
	h.mu.Unlock()
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           req.Body,
	}, nil
}

func (h *recordingHandler) sawBody() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastBody
}

func dialedController(t *testing.T, h request.Handler) *udp.Controller {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if err := router.Register(testEndpoint, h); err != nil {
		t.Fatalf("Register: %v", err)
	}
	srv, err := udp.NewServer(avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 1), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	ctrl, err := udp.NewController(avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1), srv.Addr())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	return ctrl
}

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

// REQ-DYN-008: TypedController.SendTyped encodes payload and delegates to
// inner.Request.
func TestTypedController_SendTyped(t *testing.T) {
	h := &recordingHandler{}
	inner := dialedController(t, h)

	reg := dyndata.NewRegistry()
	if err := reg.Register(eventSchema); err != nil {
		t.Fatal(err)
	}
	tc := dyndata.NewTypedController(inner, reg)
	defer func() { _ = tc.Close() }()

	p := dyndata.Payload{"zone_id": "front-left", "speed_ms": 7.2}
	resp, err := tc.SendTyped(context.Background(), testEndpoint, acf.FlagWrite, eventSchema.Name, p)
	if err != nil {
		t.Fatalf("SendTyped: %v", err)
	}
	if !resp.Control.Has(acf.FlagResponse) {
		t.Errorf("Control = %v, want FlagResponse set", resp.Control)
	}
	if len(h.sawBody()) == 0 {
		t.Error("SendTyped sent empty payload")
	}
}

func TestTypedController_SendTyped_SchemaNotFound(t *testing.T) {
	inner := dialedController(t, &recordingHandler{})

	tc := dyndata.NewTypedController(inner, dyndata.NewRegistry())
	defer func() { _ = tc.Close() }()

	_, err := tc.SendTyped(context.Background(), testEndpoint, acf.FlagWrite, "missing", nil)
	if !errors.Is(err, dyndata.ErrSchemaNotFound) {
		t.Errorf("want ErrSchemaNotFound, got %v", err)
	}
}

func TestTypedController_ClosedIdempotent(t *testing.T) {
	inner := dialedController(t, &recordingHandler{})
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
