package rcp_test

//fusa:test REQ-CONF-001
//fusa:test REQ-CONF-002
//fusa:test REQ-CONF-003

import (
	"encoding/json"
	"testing"
	"time"

	relay "github.com/SoundMatt/RELAY/v2"
	rcp "github.com/SoundMatt/go-RCP/v9"
	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
)

// TestConformance_ResponseToMessage_MatchesGenericEnvelope pins
// ResponseToMessage() to the generic relay.Message envelope contract (spec
// §4.2/§15.7.5): Protocol, ID, Payload, and a non-zero Timestamp are all
// set from the response passed in.
//
// Historical note: through RELAY v1.14.0 (the latest tag checked at
// ROADMAP.md Milestone 59), RELAY's own published rcp-status/rcp-command
// golden vector and §15.5 canonical-type schemas (spec/vectors/
// rcp-status.json, spec/schemas/rcp-{command,status}.json) still described
// the retired bespoke Zone/Command/Response/Status protocol this module no
// longer implements, so this test deliberately did not pin against them
// (see .github/workflows/ci.yml's relay-conform job history and Guiding
// Principle 10). RELAY v2.0 replaced those types and the golden vector with
// a real TC18-shaped rcp.Message (spec/vectors/rcp-message.json,
// spec/schemas/rcp-message.json); TestConformance_ResponseToMessage_
// MatchesGoldenVector below now pins against it directly, and
// .github/workflows/ci.yml's relay-conform job runs `relay interop --strict
// --protocol RCP` against the built go-rcp CLI in CI.
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

// TestConformance_ResponseToMessage_MatchesGoldenVector round-trips
// RELAY's own embedded "rcp-message" golden vector (spec/vectors/
// rcp-message.json, spec/schemas/rcp-message.json) through
// ResponseToMessage and confirms the output matches byte-for-byte,
// including Meta — the in-process analogue of what CI's relay-conform job
// checks externally via `relay interop --strict --protocol RCP` against
// the built go-rcp CLI (cmd/go-rcp/main.go's convertResponse does the same
// conversion cmd/go-rcp/main.go's own acfControlFromRELAY documents: the
// vector's flat "control" byte uses RELAY's own bit assignment, which
// differs from acf.ControlFlags's, so it is translated by flag name here
// too rather than cast directly).
//
//fusa:test REQ-CONF-001
func TestConformance_ResponseToMessage_MatchesGoldenVector(t *testing.T) {
	raw, err := relay.Vector("rcp-message")
	if err != nil {
		t.Fatalf("relay.Vector(%q): %v", "rcp-message", err)
	}

	var vector struct {
		Value struct {
			ByteBusID         int    `json:"byte_bus_id"`
			TransactionNum    uint16 `json:"transaction_num"`
			Control           int    `json:"control"`
			ReadSizeOrSegment uint16 `json:"read_size_or_segment"`
			Body              []byte `json:"body"`
		} `json:"value"`
		Message struct {
			Protocol int               `json:"protocol"`
			ID       string            `json:"id"`
			Payload  []byte            `json:"payload"`
			Meta     map[string]string `json:"meta"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatalf("unmarshal %q vector: %v", "rcp-message", err)
	}

	// RELAY's Control byte assignment (spec §15.5): Ack=0x80, Read=0x40,
	// Write=0x20, Response=0x10, Error=0x08, MoreSegments=0x04 — see
	// cmd/go-rcp/main.go's acfControlFromRELAY, duplicated narrowly here
	// since that function is unexported in package main.
	var control acf.ControlFlags
	if vector.Value.Control&0x40 != 0 {
		control |= acf.FlagRead
	}
	if vector.Value.Control&0x20 != 0 {
		control |= acf.FlagWrite
	}
	if vector.Value.Control&0x10 != 0 {
		control |= acf.FlagResponse
	}
	if vector.Value.Control&0x08 != 0 {
		control |= acf.FlagError
	}
	if vector.Value.Control&0x04 != 0 {
		control |= acf.FlagMoreSegments
	}

	got := rcp.ResponseToMessage(avtp.ByteBusID(vector.Value.ByteBusID), acf.Message{
		TransactionNum:    avtp.TransactionNum(vector.Value.TransactionNum),
		Control:           control,
		ReadSizeOrSegment: vector.Value.ReadSizeOrSegment,
		Body:              vector.Value.Body,
	})

	if int(got.Protocol) != vector.Message.Protocol {
		t.Errorf("Protocol = %d, want %d", got.Protocol, vector.Message.Protocol)
	}
	if got.ID != vector.Message.ID {
		t.Errorf("ID = %q, want %q", got.ID, vector.Message.ID)
	}
	if string(got.Payload) != string(vector.Message.Payload) {
		t.Errorf("Payload = %q, want %q", got.Payload, vector.Message.Payload)
	}
	for k, want := range vector.Message.Meta {
		if got := got.Meta[k]; got != want {
			t.Errorf("Meta[%q] = %q, want %q", k, got, want)
		}
	}
	if len(got.Meta) != len(vector.Message.Meta) {
		t.Errorf("Meta has %d keys, want %d (got %v, want %v)", len(got.Meta), len(vector.Message.Meta), got.Meta, vector.Message.Meta)
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
