//fusa:test REQ-RCS-020

package server_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/regmap"
	"github.com/SoundMatt/go-RCP/server"
)

// ── REQ-RCS-020 (golden-vector half): frozen whole-register-map byte layout ──
//
// This fixture pins the exact wire bytes EncodeRegisterMap produces for one
// representative, minimal configuration, so later milestones (Discovery
// v0.59.0's register-0 read, and every endpoint-type phase after it) can
// regression-test against a frozen encoding rather than re-deriving it from
// EncodeRegisterMap's current behaviour. A change to this byte layout is a
// deliberate register-map break, not a refactor — it must be caught here
// first.

// goldenMinimalMap is: one declared GPIO endpoint (address 1, functional
// data DE AD), one pin assignment (pin 5 -> endpoint 1, signal 0), stream
// limits (2 max streams, 8 max in-flight), and a queue config (flush
// threshold 4, flush time 100ms, heartbeat 1000ms).
var goldenMinimalMap = []byte{
	// GeneralBlock (21 bytes): VendorID=0, ProductID=0, version=0,
	// MaxEndpoints=0 (never set directly in this milestone — see
	// GeneralBlock's doc comment), MaxStreams=2 (mirrored from
	// StreamLimits), MaxFunctionalBlockBytes=0, then the four table
	// pointers: PinMap=21, StreamConfig=27, QueueConfig=30, EndpointTable=40.
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x15, 0x00,
	0x1B, 0x00, 0x1E, 0x00, 0x28,
	// PinMap (6 bytes, offset 21): count=1, then pin=5, endpoint=1, signal=0.
	0x00, 0x01, 0x00, 0x05, 0x01, 0x00,
	// StreamLimits (3 bytes, offset 27): MaxRequestStreams=2,
	// MaxInFlightPerStream=8.
	0x02, 0x00, 0x08,
	// QueueConfig (10 bytes, offset 30): FlushThreshold=4,
	// FlushTimeMillis=100, HeartbeatIntervalMillis=1000.
	0x00, 0x04, 0x00, 0x00, 0x00, 0x64, 0x00, 0x00, 0x03, 0xE8,
	// EndpointTable (9 bytes, offset 40): count=1, then one entry:
	// GenericEndpointBlock(address=1, type=GPIO=1, enabled=1) followed by
	// FunctionalBlock(length=2, data=DE AD).
	0x00, 0x01, 0x01, 0x01, 0x01, 0x00, 0x02, 0xDE, 0xAD,
}

func buildGoldenServer(t *testing.T) (*server.Server, avtp.StreamID) {
	t.Helper()
	s := server.NewServer()
	root := avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 0x0001)
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	if err := s.SetPinAssignment(root, regmap.PinAssignment{Pin: 5, Endpoint: 1, SignalIndex: 0}); err != nil {
		t.Fatalf("SetPinAssignment: %v", err)
	}
	if err := s.WriteFunctional(root, 1, []byte{0xDE, 0xAD}); err != nil {
		t.Fatalf("WriteFunctional: %v", err)
	}
	if err := s.SetStreamLimits(root, regmap.StreamLimits{MaxRequestStreams: 2, MaxInFlightPerStream: 8}); err != nil {
		t.Fatalf("SetStreamLimits: %v", err)
	}
	if err := s.SetQueueConfig(root, regmap.QueueConfig{FlushThreshold: 4, FlushTimeMillis: 100, HeartbeatIntervalMillis: 1000}); err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}
	return s, root
}

func TestGolden_MinimalRegisterMap(t *testing.T) {
	s, root := buildGoldenServer(t)

	got, err := s.ReadEP0(root)
	if err != nil {
		t.Fatalf("ReadEP0: %v", err)
	}
	if !bytes.Equal(got, goldenMinimalMap) {
		t.Fatalf("encoded register map changed:\n got  % X\n want % X", got, goldenMinimalMap)
	}

	m, err := regmap.DecodeRegisterMap(goldenMinimalMap)
	if err != nil {
		t.Fatalf("DecodeRegisterMap(golden): %v", err)
	}
	ep, ok := m.Endpoint(1)
	if !ok || ep.Generic.Type != regmap.EndpointTypeGPIO {
		t.Errorf("decoded golden vector mismatch: %+v", m)
	}
}
