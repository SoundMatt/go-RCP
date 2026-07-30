package regmap

import (
	"encoding/binary"
	"sort"

	"github.com/SoundMatt/go-RCP/avtp"
)

// GeneralBlock is the general server register block: the fixed magic value
// a client uses to recognize an RC Server, the protocol version this
// register-map encoding implements, server identification, the
// capability/capacity counters a client needs before it provisions the rest
// of the map, and byte-offset pointers to every other configuration table
// this and later milestones define. It is the one register block this
// package never accepts a client write against — see
// ErrGeneralBlockReadOnly.
//
// Field offsets, widths, and order were independently verified against the
// specification's own register-map table (issue go-RCP-N2-02) and are now
// authoritative, superseding this package's earlier reasoned-but-unverified
// layout (see doc.go's "note on spec fidelity"). This is a breaking
// wire-format change: a pre-fix client or server does not interoperate with
// this one.
type GeneralBlock struct {
	// Magic is the fixed signature a client reads first to recognize an RC
	// Server's general register block. DecodeGeneralBlock rejects any
	// block whose Magic does not equal GeneralBlockMagic.
	Magic uint32

	// ProtocolVersion is the version of the register-map wire encoding this
	// server implements, analogous in spirit to avtp.ProtocolVersion for
	// the wire layer beneath it. DecodeRegisterMap rejects any map whose
	// ProtocolVersion does not equal RegisterMapVersion.
	ProtocolVersion uint32

	// VendorID and DeviceID identify the server implementation.
	VendorID uint16
	DeviceID uint16

	// NumEndpoints is the number of endpoints (excluding EP0) this server
	// currently implements. EncodeRegisterMap always recomputes it from the
	// endpoints the map actually declares — a caller-supplied value is
	// never trusted at encode time, the same posture it already takes with
	// the table-pointer fields below.
	NumEndpoints uint16

	// MaxRequestStreams and MaxResponderStreams are the largest number of
	// request streams, respectively responder (response/ack) streams, this
	// server supports. MaxRequestStreams is mirrored from
	// StreamLimits.MaxRequestStreams at encode time so a reader of the
	// general block alone already knows it, the same way the pre-fix
	// MaxStreams field worked.
	MaxRequestStreams   uint8
	MaxResponderStreams uint8

	// MaxResponderQueueWords and MaxRequestQueueWords bound the
	// responder-queue, respectively request-queue, memory this server
	// offers, in 32-bit words.
	MaxResponderQueueWords uint16
	MaxRequestQueueWords   uint16

	// NumSequencerStates is the number of available sequencer state
	// registers; zero means sequencers are unsupported.
	NumSequencerStates uint8

	// ConfigLock is nonzero when write access to lockable parameters is
	// currently rejected; zero (the wire value 0x00) means such writes are
	// allowed.
	ConfigLock uint8

	// Options is the implemented-options bitfield: bit a (0x01) compound &
	// wait requests, bit b (0x02) trigger requests, bit c (0x04) chained
	// requests, bit d (0x08) time-synch/timed requests, bit e (0x10)
	// enhanced request cancellation.
	Options uint8

	// reserved0 is always 0x00 on the wire (offset 0x0017).
	reserved0 uint8

	// NumIOPins is the number of I/O pins assignable via the pin-mapping
	// table.
	NumIOPins uint16

	// HWConfigPointer is the byte offset, from the start of a whole
	// EP0-encoded register map, to the HW-config register map. This
	// package's own PinMap table is encoded at that offset — see
	// EncodeRegisterMap.
	HWConfigPointer uint16

	// MaxConfigurableRequestStreams and MaxConfigurableResponseStreams are
	// the largest number of configurable request streams/RC Clients,
	// respectively configurable response (and acknowledge) streams, this
	// server accepts.
	MaxConfigurableRequestStreams  uint8
	MaxConfigurableResponseStreams uint8

	// ClientConfigPointer is the byte offset to the Client-config register
	// map. This package's own StreamLimits table is encoded at that
	// offset — see EncodeRegisterMap.
	ClientConfigPointer uint16

	// QueueConfigPointer is the byte offset to the Queue/response-stream-
	// config register map. This package's own QueueConfig table is encoded
	// at that offset — see EncodeRegisterMap.
	QueueConfigPointer uint16

	// reserved1 is always 0x0000 on the wire (offset 0x0022).
	reserved1 uint16

	// EndpointConfigPointer and EndpointConfigLength are the byte offset
	// to, and byte length of, the generic part of the endpoint config
	// register map. This package's own combined per-endpoint table
	// (GenericEndpointBlock+FunctionalBlock for every declared endpoint) is
	// encoded at that offset — see EncodeRegisterMap and the judgment-call
	// note there about not yet splitting that table into the specification's
	// separate generic/mapping/functional sections.
	EndpointConfigPointer uint16
	EndpointConfigLength  uint16

	// EndpointMapPointer and EndpointMapMaxEntries describe the
	// endpoint-to-byte_bus_id mapping table. Not yet populated by this
	// package (see EncodeRegisterMap's judgment-call note) — both are
	// always zero on encode.
	EndpointMapPointer    uint16
	EndpointMapMaxEntries uint8

	// reserved2 is always 0x00 on the wire (offset 0x002B).
	reserved2 uint8

	// EndpointFunctionalConfigPointer is the byte offset to the endpoint
	// functional-config register map. Not yet populated by this package
	// (see EncodeRegisterMap's judgment-call note) — always zero on encode.
	EndpointFunctionalConfigPointer uint16

	// SequencerStateMapPointer is the byte offset to the sequencer-state
	// register map. Always zero on encode: NumSequencerStates is never set
	// above zero by this package today.
	SequencerStateMapPointer uint16
}

// GeneralBlockMagic is the fixed signature value a client reads at the
// front of the general register block to recognize a genuine RC Server.
// DecodeGeneralBlock rejects any block whose Magic field does not equal
// this constant (ErrBadMagic).
const GeneralBlockMagic uint32 = 0x52435030 // "RCP0"

// RegisterMapVersion is the only GeneralBlock.ProtocolVersion this package
// accepts on decode.
const RegisterMapVersion uint32 = 0

// sameGeneralIdentity reports whether a and b agree on every field of
// GeneralBlock that is genuinely server-owned identity/capability data.
// NumEndpoints and MaxRequestStreams (both recomputed from the rest of the
// map's actual content, mirroring the pre-fix MaxStreams field) and every
// table-pointer/length field are deliberately excluded: all of them are
// recomputed by EncodeRegisterMap, so they legitimately differ across an
// otherwise-permitted configuration write — they are not identity fields a
// client could "change" independently of the tables they describe.
func sameGeneralIdentity(a, b GeneralBlock) bool {
	return a.Magic == b.Magic &&
		a.VendorID == b.VendorID &&
		a.DeviceID == b.DeviceID &&
		a.ProtocolVersion == b.ProtocolVersion &&
		a.MaxResponderStreams == b.MaxResponderStreams &&
		a.MaxResponderQueueWords == b.MaxResponderQueueWords &&
		a.MaxRequestQueueWords == b.MaxRequestQueueWords &&
		a.NumSequencerStates == b.NumSequencerStates &&
		a.ConfigLock == b.ConfigLock &&
		a.Options == b.Options &&
		a.NumIOPins == b.NumIOPins &&
		a.MaxConfigurableRequestStreams == b.MaxConfigurableRequestStreams &&
		a.MaxConfigurableResponseStreams == b.MaxConfigurableResponseStreams
}

const generalBlockLen = 4 + 4 + 2 + 2 + 2 + 1 + 1 + 2 + 2 + 1 + 1 + 1 + 1 + 2 + 2 + 1 + 1 + 2 + 2 + 2 + 2 + 2 + 2 + 1 + 1 + 2 + 2 // 48 bytes

// EncodeGeneralBlock serializes g into its wire representation.
func EncodeGeneralBlock(g GeneralBlock) []byte {
	buf := make([]byte, generalBlockLen)
	binary.BigEndian.PutUint32(buf[0x00:0x04], g.Magic)
	binary.BigEndian.PutUint32(buf[0x04:0x08], g.ProtocolVersion)
	binary.BigEndian.PutUint16(buf[0x08:0x0A], g.VendorID)
	binary.BigEndian.PutUint16(buf[0x0A:0x0C], g.DeviceID)
	binary.BigEndian.PutUint16(buf[0x0C:0x0E], g.NumEndpoints)
	buf[0x0E] = g.MaxRequestStreams
	buf[0x0F] = g.MaxResponderStreams
	binary.BigEndian.PutUint16(buf[0x10:0x12], g.MaxResponderQueueWords)
	binary.BigEndian.PutUint16(buf[0x12:0x14], g.MaxRequestQueueWords)
	buf[0x14] = g.NumSequencerStates
	buf[0x15] = g.ConfigLock
	buf[0x16] = g.Options
	buf[0x17] = g.reserved0
	binary.BigEndian.PutUint16(buf[0x18:0x1A], g.NumIOPins)
	binary.BigEndian.PutUint16(buf[0x1A:0x1C], g.HWConfigPointer)
	buf[0x1C] = g.MaxConfigurableRequestStreams
	buf[0x1D] = g.MaxConfigurableResponseStreams
	binary.BigEndian.PutUint16(buf[0x1E:0x20], g.ClientConfigPointer)
	binary.BigEndian.PutUint16(buf[0x20:0x22], g.QueueConfigPointer)
	binary.BigEndian.PutUint16(buf[0x22:0x24], g.reserved1)
	binary.BigEndian.PutUint16(buf[0x24:0x26], g.EndpointConfigPointer)
	binary.BigEndian.PutUint16(buf[0x26:0x28], g.EndpointConfigLength)
	binary.BigEndian.PutUint16(buf[0x28:0x2A], g.EndpointMapPointer)
	buf[0x2A] = g.EndpointMapMaxEntries
	buf[0x2B] = g.reserved2
	binary.BigEndian.PutUint16(buf[0x2C:0x2E], g.EndpointFunctionalConfigPointer)
	binary.BigEndian.PutUint16(buf[0x2E:0x30], g.SequencerStateMapPointer)
	return buf
}

// DecodeGeneralBlock parses a GeneralBlock from the front of b and returns
// it along with the remaining bytes. It never panics on malformed input. A
// block whose Magic field does not equal GeneralBlockMagic is rejected with
// ErrBadMagic before any other field is interpreted.
func DecodeGeneralBlock(b []byte) (GeneralBlock, []byte, error) {
	if len(b) < generalBlockLen {
		return GeneralBlock{}, nil, ErrShortBuffer
	}
	magic := binary.BigEndian.Uint32(b[0x00:0x04])
	if magic != GeneralBlockMagic {
		return GeneralBlock{}, nil, ErrBadMagic
	}
	g := GeneralBlock{
		Magic:                           magic,
		ProtocolVersion:                 binary.BigEndian.Uint32(b[0x04:0x08]),
		VendorID:                        binary.BigEndian.Uint16(b[0x08:0x0A]),
		DeviceID:                        binary.BigEndian.Uint16(b[0x0A:0x0C]),
		NumEndpoints:                    binary.BigEndian.Uint16(b[0x0C:0x0E]),
		MaxRequestStreams:               b[0x0E],
		MaxResponderStreams:             b[0x0F],
		MaxResponderQueueWords:          binary.BigEndian.Uint16(b[0x10:0x12]),
		MaxRequestQueueWords:            binary.BigEndian.Uint16(b[0x12:0x14]),
		NumSequencerStates:              b[0x14],
		ConfigLock:                      b[0x15],
		Options:                         b[0x16],
		reserved0:                       b[0x17],
		NumIOPins:                       binary.BigEndian.Uint16(b[0x18:0x1A]),
		HWConfigPointer:                 binary.BigEndian.Uint16(b[0x1A:0x1C]),
		MaxConfigurableRequestStreams:   b[0x1C],
		MaxConfigurableResponseStreams:  b[0x1D],
		ClientConfigPointer:             binary.BigEndian.Uint16(b[0x1E:0x20]),
		QueueConfigPointer:              binary.BigEndian.Uint16(b[0x20:0x22]),
		reserved1:                       binary.BigEndian.Uint16(b[0x22:0x24]),
		EndpointConfigPointer:           binary.BigEndian.Uint16(b[0x24:0x26]),
		EndpointConfigLength:            binary.BigEndian.Uint16(b[0x26:0x28]),
		EndpointMapPointer:              binary.BigEndian.Uint16(b[0x28:0x2A]),
		EndpointMapMaxEntries:           b[0x2A],
		reserved2:                       b[0x2B],
		EndpointFunctionalConfigPointer: binary.BigEndian.Uint16(b[0x2C:0x2E]),
		SequencerStateMapPointer:        binary.BigEndian.Uint16(b[0x2E:0x30]),
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
// reject until a caller sets them plausibly). General.Magic and
// General.ProtocolVersion are pre-set to the values EncodeRegisterMap always
// forces on encode (GeneralBlockMagic and RegisterMapVersion respectively),
// so a freshly built map's stored General already agrees with what any
// encode/decode round trip of it will produce — the same reasoning that
// keeps WriteEP0's SameGeneralIdentity check from spuriously rejecting a
// legitimate configuration write that never touched either field.
func NewRegisterMap() *RegisterMap {
	return &RegisterMap{
		General:   GeneralBlock{Magic: GeneralBlockMagic, ProtocolVersion: RegisterMapVersion},
		endpoints: make(map[avtp.ByteBusID]*EndpointRegisters),
	}
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
// NumEndpoints, MaxRequestStreams (a mirror of
// StreamLimits.MaxRequestStreams), and every table-pointer/length field are
// deliberately excluded: all of them are recomputed by EncodeRegisterMap
// from the rest of the map's actual content, so they legitimately differ
// across an otherwise-permitted configuration write — they are not identity
// fields a client could "change" independently of the tables they
// describe. server.Server.WriteEP0 uses this to reject a whole-map write
// that alters the read-only general block (ErrGeneralBlockReadOnly).
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
// exposes for a whole-register-map read. GeneralBlock's Magic,
// ProtocolVersion, NumEndpoints, MaxRequestStreams, and table-pointer
// fields are always recomputed from the sections actually encoded, never
// trusted from m.General's current values (see GeneralBlock's doc comment).
//
// A judgment call: the specification's general block reserves separate
// pointer/length fields for the generic part of the endpoint config
// register map, the endpoint-to-byte_bus_id mapping table, and the
// endpoint functional-config register map. This package still encodes one
// combined per-endpoint table (GenericEndpointBlock+FunctionalBlock for
// every declared endpoint, the same layout it has always used) and points
// only EndpointConfigPointer/EndpointConfigLength at it;
// EndpointMapPointer/EndpointMapMaxEntries/EndpointFunctionalConfigPointer
// are left zero. Splitting the endpoint table into those three
// spec-distinct sections is a larger change than this fix and is left as a
// follow-on.
func EncodeRegisterMap(m *RegisterMap) []byte {
	pinBytes := encodePinMap(&m.PinMap)
	streamBytes := encodeStreamLimits(m.Streams)
	queueBytes := encodeQueueConfig(m.Queues)
	epBytes := encodeEndpointTable(m)

	general := m.General
	general.Magic = GeneralBlockMagic
	general.ProtocolVersion = RegisterMapVersion
	general.NumEndpoints = uint16(len(m.Addresses()))
	general.MaxRequestStreams = m.Streams.MaxRequestStreams
	general.HWConfigPointer = uint16(generalBlockLen)
	general.ClientConfigPointer = general.HWConfigPointer + uint16(len(pinBytes))
	general.QueueConfigPointer = general.ClientConfigPointer + uint16(len(streamBytes))
	general.EndpointConfigPointer = general.QueueConfigPointer + uint16(len(queueBytes))
	general.EndpointConfigLength = uint16(len(epBytes))

	out := make([]byte, 0, int(general.EndpointConfigPointer)+len(epBytes))
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
	if general.ProtocolVersion != RegisterMapVersion {
		return nil, ErrUnsupportedRegisterMapVersion
	}

	p1, p2, p3, p4 := general.HWConfigPointer, general.ClientConfigPointer, general.QueueConfigPointer, general.EndpointConfigPointer
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
