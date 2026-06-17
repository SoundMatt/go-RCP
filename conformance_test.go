package rcp_test

//fusa:test REQ-CONF-001
//fusa:test REQ-CONF-002
//fusa:test REQ-CONF-003

import (
	"encoding/base64"
	"testing"

	relay "github.com/SoundMatt/RELAY"
	rcp "github.com/SoundMatt/go-RCP"
)

// TestConformance_StatusToMessage_MatchesGoldenVector pins Status.ToMessage()
// to the published RELAY golden vector spec/vectors/rcp-status.json
// ("RCP status for FrontLeft zone, healthy"). The Timestamp and zero-value
// Version are not part of the deterministic comparison.
//
//fusa:test REQ-CONF-001
func TestConformance_StatusToMessage_MatchesGoldenVector(t *testing.T) {
	// Golden vector "value": {zone:1, seq:3, healthy:true, payload:"AQ=="}.
	s := &rcp.Status{
		Zone:    rcp.ZoneFrontLeft,
		Seq:     3,
		Healthy: true,
		Payload: []byte{0x01},
	}
	msg := s.ToMessage()

	// Golden vector "message".
	if msg.Protocol != relay.RCP {
		t.Errorf("protocol = %d, want %d (relay.RCP)", msg.Protocol, relay.RCP)
	}
	if msg.ID != "FrontLeft" {
		t.Errorf("id = %q, want %q", msg.ID, "FrontLeft")
	}
	if msg.Seq != 3 {
		t.Errorf("seq = %d, want 3", msg.Seq)
	}
	if got := msg.Meta["rcp.healthy"]; got != "true" {
		t.Errorf("meta[rcp.healthy] = %q, want %q", got, "true")
	}
	if got := base64.StdEncoding.EncodeToString(msg.Payload); got != "AQ==" {
		t.Errorf("payload (base64) = %q, want %q", got, "AQ==")
	}
	// The Version field defaults to its zero value, matching the vector's {0,0,0}.
	if (msg.Version != relay.Version{}) {
		t.Errorf("version = %+v, want zero value", msg.Version)
	}
}

// TestConformance_CanonicalSchemasReachable confirms go-RCP builds against a
// RELAY module that publishes the §15 canonical-type schemas (spec v0.3).
//
//fusa:test REQ-CONF-002
func TestConformance_CanonicalSchemasReachable(t *testing.T) {
	for _, name := range []string{"rcp-command", "rcp-status"} {
		b, err := relay.Schema(name)
		if err != nil {
			t.Errorf("relay.Schema(%q): %v", name, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("relay.Schema(%q) returned empty schema", name)
		}
	}
}

// TestConformance_SpecVersionTracksRELAY verifies the exported spec version is
// sourced from the RELAY module rather than hardcoded (spec §17.12 / §19.4).
//
//fusa:test REQ-CONF-003
func TestConformance_SpecVersionTracksRELAY(t *testing.T) {
	if rcp.SpecVersion != relay.SpecVersion {
		t.Errorf("rcp.SpecVersion = %q, want relay.SpecVersion %q", rcp.SpecVersion, relay.SpecVersion)
	}
	if rcp.SpecVersion == "" {
		t.Error("rcp.SpecVersion is empty")
	}
}
