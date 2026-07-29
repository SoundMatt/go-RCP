package faultinject

import (
	"fmt"
	"sync"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/discovery"
	"github.com/SoundMatt/go-RCP/e2e"
	"github.com/SoundMatt/go-RCP/request"
)

// FaultType describes the failure mode to inject. See doc.go for how each
// value maps onto this repo's real TC18 safety mechanisms.
type FaultType uint8

const (
	// FaultDrop returns an error from HandleRequest without forwarding to
	// the wrapped Handler.
	FaultDrop FaultType = iota + 1
	// FaultSlow sleeps Rule.Latency before forwarding to the wrapped Handler.
	FaultSlow
	// FaultCRCFailure returns e2e.ErrCRCMismatch without forwarding to the
	// wrapped Handler.
	FaultCRCFailure
	// FaultSafeStateEntry returns request.ErrPurgedByWatchdog without
	// forwarding to the wrapped Handler.
	FaultSafeStateEntry
	// FaultDiscoveryClaimTimeout returns
	// discovery.ErrNotConfigurationClaimant without forwarding to the
	// wrapped Handler.
	FaultDiscoveryClaimTimeout
	// FaultCancellation returns request.ErrTicketCancelled without
	// forwarding to the wrapped Handler.
	FaultCancellation
)

// Rule describes a single fault injection rule.
// Count controls how many times the rule fires: -1 means indefinitely, >0 means
// exactly that many times then the rule is automatically cleared.
type Rule struct {
	Type    FaultType
	Latency time.Duration // used by FaultSlow
	Count   int           // -1 = forever; > 0 = fires Count times then auto-removed
	fired   int
}

// Handler wraps any request.Handler and intercepts HandleRequest calls.
// Rules are applied in order; the first matching (unexpired) rule wins. It
// implements request.Handler itself, so it is directly registrable into a
// *udp.Router in place of the endpoint it wraps. All exported methods are
// safe for concurrent use.
type Handler struct {
	inner request.Handler
	mu    sync.Mutex
	rules []*Rule
}

// NewHandler wraps inner with fault injection support.
// Call AddRule to install faults before requests are handled.
func NewHandler(inner request.Handler) *Handler {
	return &Handler{inner: inner}
}

// AddRule appends a fault rule. Rules are evaluated in insertion order; the
// first active rule wins. Thread-safe.
func (h *Handler) AddRule(r Rule) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rules = append(h.rules, &r)
}

// ClearRules removes all active fault rules. Subsequent HandleRequest calls
// go straight to the wrapped Handler. Thread-safe.
func (h *Handler) ClearRules() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rules = nil
}

// HandleRequest applies the first active Rule to req and either handles it
// locally (every FaultType except FaultSlow) or forwards after a delay
// (FaultSlow). With no active rules HandleRequest delegates directly to
// inner.
func (h *Handler) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	rule := h.pickRule()
	if rule == nil {
		return h.inner.HandleRequest(requester, req)
	}
	switch rule.Type {
	case FaultDrop:
		return acf.Message{}, fmt.Errorf("rcp/faultinject: injected drop fault")
	case FaultSlow:
		if rule.Latency > 0 {
			time.Sleep(rule.Latency)
		}
		return h.inner.HandleRequest(requester, req)
	case FaultCRCFailure:
		return acf.Message{}, fmt.Errorf("rcp/faultinject: injected CRC failure: %w", e2e.ErrCRCMismatch)
	case FaultSafeStateEntry:
		return acf.Message{}, fmt.Errorf("rcp/faultinject: injected safe-state entry: %w", request.ErrPurgedByWatchdog)
	case FaultDiscoveryClaimTimeout:
		return acf.Message{}, fmt.Errorf("rcp/faultinject: injected discovery-claim timeout: %w", discovery.ErrNotConfigurationClaimant)
	case FaultCancellation:
		return acf.Message{}, fmt.Errorf("rcp/faultinject: injected cancellation: %w", request.ErrTicketCancelled)
	default:
		return h.inner.HandleRequest(requester, req)
	}
}

// pickRule returns the first active rule and updates its fire count.
// Returns nil if no rules are active.
func (h *Handler) pickRule() *Rule {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, r := range h.rules {
		if r.Count > 0 && r.fired >= r.Count {
			continue // exhausted
		}
		r.fired++
		if r.Count > 0 && r.fired >= r.Count {
			// auto-remove exhausted rule
			h.rules = append(h.rules[:i], h.rules[i+1:]...)
		}
		return r
	}
	return nil
}
