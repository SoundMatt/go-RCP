package rcp_test

//fusa:test REQ-CONF-001
//fusa:test REQ-CONF-002
//fusa:test REQ-CONF-003

import (
	"testing"
	"time"

	relay "github.com/SoundMatt/RELAY"
	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/acf"
)

// TestConformance_ResponseToMessage_MatchesGenericEnvelope pins
// ResponseToMessage() to the generic relay.Message envelope contract (spec
// §4.2/§15.7.5): Protocol, ID, Payload, and a non-zero Timestamp are all
// set from the response passed in.
//
// This does NOT pin against the RELAY module's own published rcp-status/
// rcp-command golden vector and §15.5 canonical-type schemas
// (spec/vectors/rcp-status.json, spec/schemas/rcp-{command,status}.json) —
// as of RELAY v1.14.0 (the latest tag checked at ROADMAP.md Milestone 59),
// those still describe the retired bespoke Zone/Command/Response/Status
// protocol this module no longer implements. Updating them is an upstream
// RELAY spec change, out of this repo's scope; see .github/workflows/ci.yml's
// relay-conform job for where this gap is tracked rather than silently
// glossed over (Guiding Principle 10).
//
//fusa:test REQ-CONF-001
func TestConformance_ResponseToMessage_MatchesGenericEnvelope(t *testing.T) {
	resp := acf.Message{Control: acf.FlagResponse, Body: []byte{0x01}}
	before := time.Now()
	msg := rcp.ResponseToMessage(1, resp)

	if msg.Protocol != relay.RCP {
		t.Errorf("protocol = %d, want %d (relay.RCP)", msg.Protocol, relay.RCP)
	}
	if msg.ID != rcp.EndpointIDString(1) {
		t.Errorf("id = %q, want %q", msg.ID, rcp.EndpointIDString(1))
	}
	if string(msg.Payload) != string(resp.Body) {
		t.Errorf("payload = %q, want %q", msg.Payload, resp.Body)
	}
	if msg.Timestamp.Before(before) {
		t.Errorf("timestamp = %v, want >= %v", msg.Timestamp, before)
	}
	if got := msg.Meta["rcp.error"]; got != "false" {
		t.Errorf("meta[rcp.error] = %q, want %q", got, "false")
	}
}

// TestConformance_CLISchemasReachable confirms go-RCP builds against a
// RELAY module that publishes the §12 CLI-document JSON schemas the
// go-rcp binary's version/capabilities/status commands are validated
// against by `relay conform` (see .github/workflows/ci.yml's relay-conform
// job).
//
//fusa:test REQ-CONF-002
func TestConformance_CLISchemasReachable(t *testing.T) {
	for _, name := range []string{"cli-version", "cli-capabilities", "cli-status"} {
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
