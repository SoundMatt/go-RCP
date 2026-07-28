package gpio

import "github.com/SoundMatt/go-RCP/server"

// MaxPins is the largest number of independently configured pins one GPIO
// endpoint may declare, per ROADMAP.md Milestone 47.
const MaxPins = 32

// EndpointType re-exports server.EndpointTypeGPIO so a caller that only
// imports this package doesn't also need to import server just to declare a
// GPIO endpoint's type with server.Server.AddEndpoint.
const EndpointType = server.EndpointTypeGPIO

// WriteSemantic selects how an incoming GPIO write request's operand bitmask
// combines with the endpoint's current pin state. See doc.go's spec-fidelity
// note for why this package defines exactly these eight values.
type WriteSemantic uint8

const (
	// SemanticReplace sets the addressed output pins directly to the
	// operand, discarding their previous value.
	SemanticReplace WriteSemantic = iota

	// SemanticOr sets any addressed output pin whose operand bit is 1,
	// leaving every other addressed output pin unchanged (bitwise OR).
	SemanticOr

	// SemanticAnd clears any addressed output pin whose operand bit is 0,
	// leaving every other addressed output pin unchanged (bitwise AND).
	SemanticAnd

	// SemanticAndNot clears any addressed output pin whose operand bit is 1,
	// leaving every other addressed output pin unchanged (bitwise AND NOT /
	// clear-mask) — the natural complement to SemanticOr's set-mask.
	SemanticAndNot

	// SemanticXor toggles any addressed output pin whose operand bit is 1,
	// leaving every other addressed output pin unchanged (bitwise XOR).
	SemanticXor

	// SemanticSaturatingAdd treats the current output word and the operand
	// as unsigned integers and adds them, clamping at the active-pin mask
	// ((1<<PinCount)-1) instead of wrapping.
	SemanticSaturatingAdd

	// SemanticSaturatingSubtract treats the current output word and the
	// operand as unsigned integers and subtracts, clamping at zero instead
	// of wrapping.
	SemanticSaturatingSubtract

	// SemanticReconfigure is the escape hatch: instead of combining with the
	// pin value, the operand replaces Config.Direction (masked to the
	// endpoint's active pins), and the change is persisted back through the
	// register map. The pin value itself is left untouched.
	SemanticReconfigure

	semanticCount // sentinel; keep last
)

// Valid reports whether s is one of this package's eight recognized
// write-semantic values.
func (s WriteSemantic) Valid() bool {
	return s < semanticCount
}
