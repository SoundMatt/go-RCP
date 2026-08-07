package adc

import (
	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/regmap"
)

// EndpointType re-exports regmap.EndpointTypeADC so a caller that only
// imports this package doesn't also need to import server just to declare an
// ADC endpoint's type with server.Server.AddEndpoint.
const EndpointType = regmap.EndpointTypeADC

// EVTClass is the row of TC18 §13.5 Table 30 that governs how this endpoint
// type interprets a request's evt[2:0] field. ADC sits in the row Table 30
// shares between ADC, PWM_IN, I²C, LIN, CAN, UART, ISELED and MDIO: the row
// that defines no interface-combining semantics at all, whose only special
// selector is the §12.7.1 configuration change at 111b.
// Endpoint.HandleRequest routes every request through
// acf.Message.EVTDisposition with this class rather than ignoring evt
// entirely — see acf/evt.go, including its documented reading of this row's
// 000b entry.
const EVTClass = acf.EVTClassConfigOnly

// MaxResolutionBits is the largest sample resolution one ADC endpoint may
// declare, per ROADMAP.md Milestone 48.
const MaxResolutionBits = 16

// CombineMode selects how a measurement's averaged sample combines with the
// endpoint's previously reported value to produce the value HandleRequest
// reports — the third of this package's three-layer sample/average/combine
// model (see doc.go's Scope section).
type CombineMode uint8

const (
	// CombineReplace discards the endpoint's previous value: the reported
	// value is exactly this measurement's averaged sample.
	CombineReplace CombineMode = iota

	// CombineRollingAverage blends the endpoint's previous value and this
	// measurement's averaged sample in equal parts (a simple one-pole
	// smoothing combine), rounding down.
	CombineRollingAverage

	combineModeCount // sentinel; keep last
)

// Valid reports whether m is one of this package's two recognized combine
// modes.
func (m CombineMode) Valid() bool {
	return m < combineModeCount
}

// TriggerMode selects how this ADC channel is kept sampling continuously,
// since (per doc.go's Scope section) the endpoint never samples on its own.
// This is metadata a driving caller consults — Endpoint.Trigger performs one
// measurement regardless of TriggerMode; the mode only documents which of
// the two continuous-sampling mechanisms a caller is expected to be using to
// invoke it repeatedly, and shapes how a plain read request behaves (see
// Endpoint.HandleRequest).
type TriggerMode uint8

const (
	// TriggerModeOnDemand means nothing drives this channel continuously: a
	// plain read request itself performs one fresh measurement.
	TriggerModeOnDemand TriggerMode = iota

	// TriggerModeExternal means a caller is expected to invoke
	// Endpoint.Trigger once per edge it drains from another endpoint's own
	// trigger queue (the same TriggerEvent/DrainTriggers mechanism spi
	// introduced in v0.60.0) — a plain read request just returns the latest
	// already-measured value without forcing a fresh synchronous sample.
	TriggerModeExternal

	// TriggerModeSelf means a caller is expected to invoke Endpoint.Trigger
	// once per TriggerMeasurementDone event this endpoint's own
	// DrainTriggers reports, chaining continuous sampling off its own
	// completion signal — a plain read request behaves the same as
	// TriggerModeExternal.
	TriggerModeSelf

	triggerModeCount // sentinel; keep last
)

// Valid reports whether m is one of this package's three recognized trigger
// modes.
func (m TriggerMode) Valid() bool {
	return m < triggerModeCount
}
