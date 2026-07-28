package regmap

import (
	"encoding/binary"
	"sort"

	"github.com/SoundMatt/go-RCP/avtp"
)

// GeneralBlock is the general server register block: server identification,
// the protocol version this register-map encoding implements, the
// capability/capacity counters a client needs before it provisions the rest
// of the map, and byte-offset pointers to every other configuration table
// this and later milestones define. It is the one register block this
// package never accepts a client write against — see
// ErrGeneralBlockReadOnly.
type GeneralBlock struct {
	// VendorID and ProductID identify the server implementation.
	VendorID  uint32
	ProductID uint32

	// RegisterMapVersion is the version of this package's register-map
	// encoding, analogous in spirit to avtp.ProtocolVersion for the wire
	// layer beneath it.
	RegisterMapVersion uint8

	// MaxEndpoints is the largest number of endpoints (excluding EP0) this
	// server supports declaring.
	MaxEndpoints uint8

	// MaxStreams is the largest number of concurrent request streams this
	// server supports, mirrored from StreamLimits.MaxRequestStreams at
	// encode time so a reader of the general block alone already knows it.
	MaxStreams uint8

	// MaxFunctionalBlockBytes bounds how large any single endpoint's
	// functional (type-specific) configuration block may be.
	MaxFunctionalBlockBytes uint16

	// PinMapPointer, StreamConfigPointer, QueueConfigPointer, and
	// EndpointTablePointer are byte offsets, from the start of a whole
	// EP0-encoded register map, to each of those tables.
	//
	// EncodeRegisterMap always recomputes these four fields from the
	// tables it is actually given — a caller-supplied value is never
	// trusted at encode time, the same posture acf.EncodeFrame takes with
	// Header.DataLength.
	PinMapPointer        uint16
	StreamConfigPointer  uint16
	QueueConfigPointer   uint16
	EndpointTablePointer uint16
}

// RegisterMapVersion is the only GeneralBlock.RegisterMapVersion this
// package accepts on decode.
const RegisterMapVersion uint8 = 0

// sameGeneralIdentity reports whether a and b agree on every field of
// GeneralBlock that is genuinely server-owned identity/capability data.
// MaxStreams (a mirror of StreamLimits.MaxRequestStreams) and the four
// table pointers are deliberately excluded: both are recomputed by
// EncodeRegisterMap from the rest of the map's actual content, so they
// legitimately differ across an otherwise-permitted configuration write —
// they are not identity fields a client could "change" independently of the
// tables they describe.
func sameGeneralIdentity(a, b GeneralBlock) bool {
	return a.VendorID == b.VendorID &&
		a.ProductID == b.ProductID &&
		a.RegisterMapVersion == b.RegisterMapVersion &&
		a.MaxEndpoints == b.MaxEndpoints &&
		a.MaxFunctionalBlockBytes == b.MaxFunctionalBlockBytes
}

const generalBlockLen = 4 + 4 + 1 + 1 + 1 + 2 + 2 + 2 + 2 + 2 // 21 bytes

// EncodeGeneralBlock serializes g into its wire representation.
func EncodeGeneralBlock(g GeneralBlock) []byte {
	buf := make([]byte, generalBlockLen)
	binary.BigEndian.PutUint32(buf[0:4], g.VendorID)
	binary.BigEndian.PutUint32(buf[4:8], g.ProductID)
	buf[8] = g.RegisterMapVersion
	buf[9] = g.MaxEndpoints
	buf[10] = g.MaxStreams
	binary.BigEndian.PutUint16(buf[11:13], g.MaxFunctionalBlockBytes)
	binary.BigEndian.PutUint16(buf[13:15], g.PinMapPointer)
	binary.BigEndian.PutUint16(buf[15:17], g.StreamConfigPointer)
	binary.BigEndian.PutUint16(buf[17:19], g.QueueConfigPointer)
	binary.BigEndian.PutUint16(buf[19:21], g.EndpointTablePointer)
	return buf
}

// DecodeGeneralBlock parses a GeneralBlock from the front of b and returns
// it along with the remaining bytes. It never panics on malformed input.
func DecodeGeneralBlock(b []byte) (GeneralBlock, []byte, error) {
	if len(b) < generalBlockLen {
		return GeneralBlock{}, nil, ErrShortBuffer
	}
	g := GeneralBlock{
		VendorID:                binary.BigEndian.Uint32(b[0:4]),
		ProductID:               binary.BigEndian.Uint32(b[4:8]),
		RegisterMapVersion:      b[8],
		MaxEndpoints:            b[9],
		MaxStreams:              b[10],
		MaxFunctionalBlockBytes: binary.BigEndian.Uint16(b[11:13]),
		PinMapPointer:           binary.BigEndian.Uint16(b[13:15]),
		StreamConfigPointer:     binary.BigEndian.Uint16(b[15:17]),
		QueueConfigPointer:      binary.BigEndian.Uint16(b[17:19]),
		EndpointTablePointer:    binary.BigEndian.Uint16(b[19:21]),
	}
	return g, b[generalBlockLen:], nil
}

// GenericEndpointBlock is the common/generic per-endpoint block: the fields
// the server itself owns about a declared endpoint, kept structurally
// separate from that endpoint's own type-specific FunctionalBlock.
type GenericEndpointBlock struct {
	// Address is this endpoint's byte_bus_id.
	Address avtp.ByteBusID

	// Type selects which functional endpoint kind this address was
	// declared as.
	Type EndpointType

	// Enabled reports whether the server currently offers this endpoint.
	// A declared-but-disabled endpoint keeps its address reserved (no
	// other endpoint may reuse it) without participating in discovery or
	// accepting requests; that distinction belongs to later milestones,
	// this package only carries the flag.
	Enabled bool
}

const genericEndpointBlockLen = 1 + 1 + 1 // 3 bytes

func encodeGenericEndpointBlock(g GenericEndpointBlock) []byte {
	buf := make([]byte, genericEndpointBlockLen)
	buf[0] = byte(g.Address)
	buf[1] = byte(g.Type)
	if g.Enabled {
		buf[2] = 1
	}
	return buf
}

func decodeGenericEndpointBlock(b []byte) (GenericEndpointBlock, []byte, error) {
	if len(b) < genericEndpointBlockLen {
		return GenericEndpointBlock{}, nil, ErrShortBuffer
	}
	t := EndpointType(b[1])
	if t >= endpointTypeCount {
		return GenericEndpointBlock{}, nil, ErrUnknownEndpointType
	}
	g := GenericEndpointBlock{
		Address: avtp.ByteBusID(b[0]),
		Type:    t,
		Enabled: b[2] != 0,
	}
	return g, b[genericEndpointBlockLen:], nil
}

// FunctionalBlock is an endpoint's own type-specific functional
// configuration block, kept structurally separate from the server-owned
// GenericEndpointBlock. Its interpretation depends on the endpoint's
// declared Type; this milestone only carries the raw bytes, since the
// per-type layouts (GPIO, SPI, I²C, ...) are later Phase 14/16 milestones
// layered on top of this register-map foundation.
type FunctionalBlock struct {
	Data []byte
}

func encodeFunctionalBlock(f FunctionalBlock) []byte {
	buf := make([]byte, 2+len(f.Data))
	binary.BigEndian.PutUint16(buf[0:2], uint16(len(f.Data)))
	copy(buf[2:], f.Data)
	return buf
}

func decodeFunctionalBlock(b []byte) (FunctionalBlock, []byte, error) {
	if len(b) < 2 {
		return FunctionalBlock{}, nil, ErrShortBuffer
	}
	n := int(binary.BigEndian.Uint16(b[0:2]))
	if len(b) < 2+n {
		return FunctionalBlock{}, nil, ErrShortBuffer
	}
	f := FunctionalBlock{}
	if n > 0 {
		f.Data = make([]byte, n)
		copy(f.Data, b[2:2+n])
	}
	return f, b[2+n:], nil
}

// EndpointRegisters is one declared endpoint's full register state: the
// server-owned generic block plus the endpoint's own functional block.
type EndpointRegisters struct {
	Generic    GenericEndpointBlock
	Functional FunctionalBlock
}

// RegisterMap is the RC Server's whole register map: the general block, the
// HW pin-mapping table, the request-stream/queue configuration tables, and
// every declared endpoint's registers. EP0 addresses this structure as a
// whole; see Server for the lifecycle-gated read/write surface over it.
type RegisterMap struct {
	General   GeneralBlock
	PinMap    PinMap
	Streams   StreamLimits
	Queues    QueueConfig
	endpoints map[avtp.ByteBusID]*EndpointRegisters
}

// NewRegisterMap returns an empty register map with bare-defaults values:
// no endpoints declared, an empty pin map, and zero-value stream/queue
// configuration (both of which Server.AdvanceToFullyConfigured's guard will
// reject until a caller sets them plausibly).
func NewRegisterMap() *RegisterMap {
	return &RegisterMap{endpoints: make(map[avtp.ByteBusID]*EndpointRegisters)}
}

// endpointType implements the endpointTypes interface PinMap.Validate uses.
func (m *RegisterMap) endpointType(addr avtp.ByteBusID) (EndpointType, bool) {
	ep, ok := m.endpoints[addr]
	if !ok {
		return EndpointTypeUnassigned, false
	}
	return ep.Generic.Type, true
}

// Endpoint returns the declared endpoint at addr, if any.
func (m *RegisterMap) Endpoint(addr avtp.ByteBusID) (*EndpointRegisters, bool) {
	ep, ok := m.endpoints[addr]
	return ep, ok
}

// Addresses returns every declared endpoint address in ascending order.
// EncodeRegisterMap lays the endpoint table out in this same order, so a
// client's endpoint-address mapping table stays consistent with it
// end-to-end. See the ROADMAP.md Milestone 45 spec-fidelity note in doc.go
// for why that ordering has no wire-format safety net of its own.
func (m *RegisterMap) Addresses() []avtp.ByteBusID {
	out := make([]avtp.ByteBusID, 0, len(m.endpoints))
	for addr := range m.endpoints {
		out = append(out, addr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// HasEndpoint reports whether an endpoint is already declared at addr.
func (m *RegisterMap) HasEndpoint(addr avtp.ByteBusID) bool {
	_, ok := m.endpoints[addr]
	return ok
}

// DeclareEndpoint declares a new, enabled endpoint of type t at addr, with
// an empty FunctionalBlock. The caller (server.Server.AddEndpoint) is
// responsible for every precondition this package itself has no way to
// check from inside RegisterMap alone: that addr is not the reserved EP0
// address, that no endpoint is already declared there (see HasEndpoint),
// and that the caller holds whatever access the surrounding lifecycle/
// access-control rules require.
func (m *RegisterMap) DeclareEndpoint(addr avtp.ByteBusID, t EndpointType) {
	m.endpoints[addr] = &EndpointRegisters{
		Generic: GenericEndpointBlock{Address: addr, Type: t, Enabled: true},
	}
}

// Encode serializes ep's generic block followed by its functional block —
// the same per-endpoint byte layout EncodeRegisterMap's endpoint table
// uses — for a caller (server.Server.ReadEndpoint) that needs just one
// endpoint's bytes rather than the whole map.
func (ep *EndpointRegisters) Encode() []byte {
	out := encodeGenericEndpointBlock(ep.Generic)
	out = append(out, encodeFunctionalBlock(ep.Functional)...)
	return out
}

// SameGeneralIdentity reports whether a and b agree on every field of
// GeneralBlock that is genuinely server-owned identity/capability data.
// MaxStreams (a mirror of StreamLimits.MaxRequestStreams) and the four
// table pointers are deliberately excluded: both are recomputed by
// EncodeRegisterMap from the rest of the map's actual content, so they
// legitimately differ across an otherwise-permitted configuration write —
// they are not identity fields a client could "change" independently of
// the tables they describe. server.Server.WriteEP0 uses this to reject a
// whole-map write that alters the read-only general block
// (ErrGeneralBlockReadOnly).
func SameGeneralIdentity(a, b GeneralBlock) bool {
	return sameGeneralIdentity(a, b)
}

// SameEndpointGenerics reports whether a and b declare exactly the same set
// of endpoint addresses with exactly the same GenericEndpointBlock at each
// one. Endpoint declaration (address/type/enabled) is locked in alongside
// the pin-mapping table once the server leaves StateUnconfigured; only each
// endpoint's FunctionalBlock, and the stream/queue tables, may still
// change. server.Server.WriteEP0 uses this to detect an attempted change to
// the locked endpoint topology once the server has left StateUnconfigured.
func SameEndpointGenerics(a, b *RegisterMap) bool {
	if len(a.endpoints) != len(b.endpoints) {
		return false
	}
	for addr, ea := range a.endpoints {
		eb, ok := b.endpoints[addr]
		if !ok || ea.Generic != eb.Generic {
			return false
		}
	}
	return true
}

func encodePinMap(p *PinMap) []byte {
	entries := p.Entries()
	buf := make([]byte, 2+4*len(entries))
	binary.BigEndian.PutUint16(buf[0:2], uint16(len(entries)))
	off := 2
	for _, a := range entries {
		binary.BigEndian.PutUint16(buf[off:off+2], a.Pin)
		buf[off+2] = byte(a.Endpoint)
		buf[off+3] = a.SignalIndex
		off += 4
	}
	return buf
}

func decodePinMap(b []byte) (PinMap, error) {
	if len(b) < 2 {
		return PinMap{}, ErrShortBuffer
	}
	n := int(binary.BigEndian.Uint16(b[0:2]))
	need := 2 + 4*n
	if len(b) != need {
		return PinMap{}, ErrShortBuffer
	}
	var p PinMap
	off := 2
	for i := 0; i < n; i++ {
		p.Set(PinAssignment{
			Pin:         binary.BigEndian.Uint16(b[off : off+2]),
			Endpoint:    avtp.ByteBusID(b[off+2]),
			SignalIndex: b[off+3],
		})
		off += 4
	}
	return p, nil
}

const streamLimitsLen = 1 + 2 // 3 bytes

func encodeStreamLimits(s StreamLimits) []byte {
	buf := make([]byte, streamLimitsLen)
	buf[0] = s.MaxRequestStreams
	binary.BigEndian.PutUint16(buf[1:3], s.MaxInFlightPerStream)
	return buf
}

func decodeStreamLimits(b []byte) (StreamLimits, error) {
	if len(b) != streamLimitsLen {
		return StreamLimits{}, ErrShortBuffer
	}
	return StreamLimits{
		MaxRequestStreams:    b[0],
		MaxInFlightPerStream: binary.BigEndian.Uint16(b[1:3]),
	}, nil
}

const queueConfigLen = 2 + 4 + 4 // 10 bytes

func encodeQueueConfig(q QueueConfig) []byte {
	buf := make([]byte, queueConfigLen)
	binary.BigEndian.PutUint16(buf[0:2], q.FlushThreshold)
	binary.BigEndian.PutUint32(buf[2:6], q.FlushTimeMillis)
	binary.BigEndian.PutUint32(buf[6:10], q.HeartbeatIntervalMillis)
	return buf
}

func decodeQueueConfig(b []byte) (QueueConfig, error) {
	if len(b) != queueConfigLen {
		return QueueConfig{}, ErrShortBuffer
	}
	return QueueConfig{
		FlushThreshold:          binary.BigEndian.Uint16(b[0:2]),
		FlushTimeMillis:         binary.BigEndian.Uint32(b[2:6]),
		HeartbeatIntervalMillis: binary.BigEndian.Uint32(b[6:10]),
	}, nil
}

func encodeEndpointTable(m *RegisterMap) []byte {
	addrs := m.Addresses()
	buf := make([]byte, 0, 2+len(addrs)*(genericEndpointBlockLen+2))
	countBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(countBuf, uint16(len(addrs)))
	buf = append(buf, countBuf...)
	for _, addr := range addrs {
		ep := m.endpoints[addr]
		buf = append(buf, encodeGenericEndpointBlock(ep.Generic)...)
		buf = append(buf, encodeFunctionalBlock(ep.Functional)...)
	}
	return buf
}

func decodeEndpointTable(b []byte) (map[avtp.ByteBusID]*EndpointRegisters, error) {
	if len(b) < 2 {
		return nil, ErrShortBuffer
	}
	n := int(binary.BigEndian.Uint16(b[0:2]))
	rest := b[2:]
	out := make(map[avtp.ByteBusID]*EndpointRegisters, n)
	for i := 0; i < n; i++ {
		generic, tail, err := decodeGenericEndpointBlock(rest)
		if err != nil {
			return nil, err
		}
		functional, tail2, err := decodeFunctionalBlock(tail)
		if err != nil {
			return nil, err
		}
		out[generic.Address] = &EndpointRegisters{Generic: generic, Functional: functional}
		rest = tail2
	}
	if len(rest) != 0 {
		return nil, ErrTrailingBytes
	}
	return out, nil
}

// EncodeRegisterMap serializes m into the single contiguous byte buffer EP0
// exposes for a whole-register-map read. GeneralBlock's four table pointers
// are always recomputed from the sections actually encoded, never trusted
// from m.General's current values (see GeneralBlock's doc comment).
func EncodeRegisterMap(m *RegisterMap) []byte {
	pinBytes := encodePinMap(&m.PinMap)
	streamBytes := encodeStreamLimits(m.Streams)
	queueBytes := encodeQueueConfig(m.Queues)
	epBytes := encodeEndpointTable(m)

	general := m.General
	general.RegisterMapVersion = RegisterMapVersion
	general.MaxStreams = m.Streams.MaxRequestStreams
	general.PinMapPointer = uint16(generalBlockLen)
	general.StreamConfigPointer = general.PinMapPointer + uint16(len(pinBytes))
	general.QueueConfigPointer = general.StreamConfigPointer + uint16(len(streamBytes))
	general.EndpointTablePointer = general.QueueConfigPointer + uint16(len(queueBytes))

	out := make([]byte, 0, int(general.EndpointTablePointer)+len(epBytes))
	out = append(out, EncodeGeneralBlock(general)...)
	out = append(out, pinBytes...)
	out = append(out, streamBytes...)
	out = append(out, queueBytes...)
	out = append(out, epBytes...)
	return out
}

// DecodeRegisterMap parses a whole register map from b, following
// GeneralBlock's own table pointers to locate each section. It never
// panics on malformed input, and rejects a buffer with any bytes left over
// once the endpoint table's own declared entry count is exhausted, the same
// posture acf.DecodeFrame takes on a length mismatch.
func DecodeRegisterMap(b []byte) (*RegisterMap, error) {
	general, _, err := DecodeGeneralBlock(b)
	if err != nil {
		return nil, err
	}
	if general.RegisterMapVersion != RegisterMapVersion {
		return nil, ErrUnsupportedRegisterMapVersion
	}

	p1, p2, p3, p4 := general.PinMapPointer, general.StreamConfigPointer, general.QueueConfigPointer, general.EndpointTablePointer
	inOrder := uint16(generalBlockLen) <= p1 && p1 <= p2 && p2 <= p3 && p3 <= p4 && int(p4) <= len(b)
	if !inOrder {
		return nil, ErrShortBuffer
	}

	pinMap, err := decodePinMap(b[p1:p2])
	if err != nil {
		return nil, err
	}
	streams, err := decodeStreamLimits(b[p2:p3])
	if err != nil {
		return nil, err
	}
	queues, err := decodeQueueConfig(b[p3:p4])
	if err != nil {
		return nil, err
	}
	endpoints, err := decodeEndpointTable(b[p4:])
	if err != nil {
		return nil, err
	}

	return &RegisterMap{
		General:   general,
		PinMap:    pinMap,
		Streams:   streams,
		Queues:    queues,
		endpoints: endpoints,
	}, nil
}
