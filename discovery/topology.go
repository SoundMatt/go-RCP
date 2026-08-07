package discovery

import (
	"encoding/json"
	"io"

	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/regmap"
)

// This file is the client-facing half of ROADMAP.md Milestone 46
// (Discovery): recognizing a conformant server from a decoded discovery
// response, and persisting the resulting topology so re-discovery isn't
// mandatory every power cycle. Full client-side configuration tooling is
// deferred to the Phase 17 control-plane migration (ROADMAP.md Milestone
// 55, see server/doc.go's endpoint-address-ordering note).

// IsConformantServer reports whether a discovery response's general block
// looks like a genuine RCP server, rather than noise from an unrelated
// device answering (or misinterpreted) on the same untimed AVTPDU stream:
// its Magic must match GeneralBlockMagic (DecodeGeneralBlock already
// enforces this before a caller ever reaches a GeneralBlock value, so this
// check is belt-and-suspenders for a value built by hand rather than
// decoded), its ProtocolVersion must match the version the regmap package
// implements, and it must carry a nonzero vendor and device
// identification. This is this implementation's own reasoned recognition
// heuristic — built from the fields regmap.GeneralBlock already carries —
// rather than a verified transcription of a published conformance test, the
// same open-item posture the rest of this package's spec-fidelity notes
// document.
func IsConformantServer(g regmap.GeneralBlock) bool {
	return g.Magic == regmap.GeneralBlockMagic &&
		g.ProtocolVersion == regmap.RegisterMapVersion &&
		g.VendorID != 0 && g.DeviceID != 0
}

// TopologyEndpoint is one declared endpoint's identity as persisted by
// Topology: enough for a client to re-address it without re-declaring its
// full functional configuration.
type TopologyEndpoint struct {
	Address avtp.ByteBusID      `json:"address"`
	Type    regmap.EndpointType `json:"type"`
}

// Topology is a client's persisted summary of one server's discovered
// register-map shape: the general block's identification/version fields,
// plus every declared endpoint's address and type. It deliberately omits
// each endpoint's FunctionalBlock, the pin-mapping table, and the
// stream/queue configuration — Topology is what re-discovery would
// otherwise re-learn (ROADMAP.md Milestone 46), not a full configuration
// backup; a client that needs the whole map again still has
// EncodeRegisterMap/DecodeRegisterMap for that.
type Topology struct {
	VendorID        uint16             `json:"vendor_id"`
	DeviceID        uint16             `json:"device_id"`
	ProtocolVersion uint32             `json:"protocol_version"`
	Endpoints       []TopologyEndpoint `json:"endpoints"`
}

// EndpointCount returns the number of endpoints t records — the "endpoint
// count" ROADMAP.md Milestone 46 lists among the fields a client uses to
// recognize and characterize a discovered server.
func (t Topology) EndpointCount() int {
	return len(t.Endpoints)
}

// DiscoverTopology extracts a client-persistable Topology summary from a
// decoded discovery response (see regmap.DecodeRegisterMap). Endpoints are
// listed in the same ascending byte_bus_id order RegisterMap.Addresses
// returns.
func DiscoverTopology(m *regmap.RegisterMap) Topology {
	addrs := m.Addresses()
	endpoints := make([]TopologyEndpoint, 0, len(addrs))
	for _, addr := range addrs {
		ep, _ := m.Endpoint(addr)
		endpoints = append(endpoints, TopologyEndpoint{Address: addr, Type: ep.Generic.Type})
	}
	return Topology{
		VendorID:        m.General.VendorID,
		DeviceID:        m.General.DeviceID,
		ProtocolVersion: m.General.ProtocolVersion,
		Endpoints:       endpoints,
	}
}

// WriteTopology persists t as JSON to w, so a client can reload it on a
// later power cycle instead of re-running discovery (ROADMAP.md Milestone
// 46). Persistence uses JSON rather than the register-map wire encoding on
// purpose: Topology is this package's own client-side bookkeeping format,
// not an on-the-wire RCP structure, the same distinction config.File draws
// against the register-map/AVTPDU encodings elsewhere in this repo.
func WriteTopology(w io.Writer, t Topology) error {
	return json.NewEncoder(w).Encode(t)
}

// ReadTopology loads a Topology previously persisted by WriteTopology.
func ReadTopology(r io.Reader) (Topology, error) {
	var t Topology
	if err := json.NewDecoder(r).Decode(&t); err != nil {
		return Topology{}, err
	}
	return t, nil
}
