package mdio

import (
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/regmap"
)

// EndpointType re-exports regmap.EndpointTypeMDIO so a caller that only
// imports this package doesn't also need to import server just to declare
// an MDIO endpoint's type with server.Server.AddEndpoint.
const EndpointType = regmap.EndpointTypeMDIO

// EVTClass is the row of TC18 §13.5 Table 30 that governs how this endpoint
// type interprets a request's evt[2:0] field. MDIO sits in the row Table 30
// shares between ADC, PWM_IN, I²C, LIN, CAN, UART, ISELED and MDIO: the row
// that defines no interface-combining semantics at all, whose only special
// selector is the §12.7.1 configuration change at 111b.
// Endpoint.HandleRequest routes every request through
// acf.Message.EVTDisposition with this class rather than ignoring evt
// entirely — see acf/evt.go, including its documented reading of this row's
// 000b entry.
const EVTClass = acf.EVTClassConfigOnly

// Mode selects which of the specification's four MDIO addressing/access
// shapes a Request uses — the wire format's 2-bit mdio_mode field. See
// doc.go's Scope section.
type Mode uint8

const (
	// ModeMMDSingleWord accesses a single word of a Clause 45-style MMD
	// (MDIO Manageable Device) register. Request.DevAddr is the address of
	// that register within the MMD, not a selector for which MMD (see
	// Request's doc comment).
	ModeMMDSingleWord Mode = iota

	// ModeMMDMultiByte accesses a Clause 45-style MMD register using the
	// specification's multiple-byte access shape rather than a single
	// fixed word. Request.DevAddr is the address of that register within
	// the MMD, same as ModeMMDSingleWord.
	ModeMMDMultiByte

	// ModeMMSSingleWord accesses a single word of an MMS (memory-mapped
	// space) register. Request.DevAddr selects which MMS; see
	// Request.DataWidth for how the selected MMS index determines this
	// access's payload width.
	ModeMMSSingleWord

	// ModeMMSMultiWord accesses an MMS register using the specification's
	// multiple (double-word) access shape. Request.DevAddr selects which
	// MMS; see Request.DataWidth for how the selected MMS index determines
	// this access's payload width.
	ModeMMSMultiWord

	modeCount // sentinel; keep last
)

// Valid reports whether m is one of this package's four recognized
// mdio_mode values.
func (m Mode) Valid() bool {
	return m < modeCount
}

// devAddrMax bounds the 5-bit device/MMS-select address field every mode
// shares. The wire field it occupies (mdio_address) is 6 bits wide (see
// request.go), but the valid value range this package enforces follows
// IEEE Clause 45's 5-bit DEVAD address space, so the wire field's top bit
// is always 0 for any address this package accepts.
const devAddrMax = 0x1F

// mmsWideIndexMax is the highest MMS index that uses a 32-bit register
// width — the two indices conventionally called "MMS0" and "MMS1" (0 and
// 1). Every other MMS index uses a 16-bit register width. See
// Request.DataWidth.
//
// This package reuses Request.DevAddr — the single wire field the
// specification's mdio_address occupies — for two different meanings
// depending on Mode: for the two MMD modes it is the address of a
// register within an (implicitly selected) MMD, per the specification's
// own MDIO request-handling description; for the two MMS modes it selects
// which MMS. Both readings share the wire's single mdio_address field,
// and the two are never both meaningful for the same Request (Mode picks
// exactly one interpretation). See doc.go's "A note on spec fidelity"
// section.
const mmsWideIndexMax = 1

// Request is one addressed MDIO register access, matching the
// specification's mdio request format: Mode selects one of this package's
// four mdio_mode access shapes (see Mode); DevAddr is the wire's single
// 6-bit mdio_address field, whose meaning depends on Mode — the address of
// a register within an (implicitly selected) MMD for the two MMD modes, or
// the target MMS index for the two MMS modes (see Mode's own doc comments
// and mmsWideIndexMax). There is no separate device-selector field and no
// separate register-address field beyond mdio_address itself — the
// specification's request-handling description states the request carries
// "the MMD and the address in the MMD" as a single concept, not two. See
// DataWidth for how Mode and DevAddr together determine this access's
// payload width.
//
// There is deliberately no separate PHY-address field either: a target PHY
// is selected by which endpoint (byte_bus_id) a request addresses, not by
// a field within the request body.
type Request struct {
	Mode    Mode
	DevAddr uint8
}

// DataWidth reports the width, in bytes, of the register-value payload
// this Request's Mode — and, for the two MMS modes, DevAddr — selects:
// MMD accesses (ModeMMDSingleWord, ModeMMDMultiByte) are always 16 bits (2
// bytes) wide. MMS accesses (ModeMMSSingleWord, ModeMMSMultiWord) are 32
// bits (4 bytes) wide when DevAddr selects MMS index 0 or 1
// ("MMS0"/"MMS1" — see mmsWideIndexMax) and 16 bits (2 bytes) wide for
// every other MMS index. An invalid Mode (see Valid) is treated as 16
// bits wide; callers should call Validate first.
func (r Request) DataWidth() int {
	switch r.Mode {
	case ModeMMSSingleWord, ModeMMSMultiWord:
		if r.DevAddr <= mmsWideIndexMax {
			return 4
		}
	}
	return 2
}

// Validate reports whether r is a plausible, encodable Request: Mode must
// be recognized, and DevAddr must fit its 5-bit width.
func (r Request) Validate() error {
	if !r.Mode.Valid() {
		return ErrInvalidMode
	}
	if r.DevAddr > devAddrMax {
		return ErrDevAddrOutOfRange
	}
	return nil
}
