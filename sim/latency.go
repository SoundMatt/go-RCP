package sim

import (
	"math/rand"
	"sync"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/request"
)

// LatencyModel selects how LatencyHandler computes each call's added delay.
type LatencyModel uint8

const (
	// LatencyConstant always adds exactly Base.
	LatencyConstant LatencyModel = iota
	// LatencyJitter adds Base plus a uniform random amount in [0, Jitter].
	LatencyJitter
)

// LatencyHandler wraps a request.Handler with a configurable simulated
// response latency and implements request.Handler itself, so it is
// directly registrable into a *udp.Router in place of the endpoint it
// wraps — the same "wrap, don't require call-site changes" posture
// e2e.Guard, proxy.Handler, and record.Handler (this milestone) already
// establish for this repo's shared request.Handler interface.
// request.Handler carries no context.Context a caller could cancel this
// delay through (every Phase 14/16 endpoint's own HandleRequest is
// synchronous), so the added delay is a real time.Sleep, not a select
// against a deadline. All exported methods are safe for concurrent use.
type LatencyHandler struct {
	inner  request.Handler
	base   time.Duration
	jitter time.Duration
	model  LatencyModel

	mu  sync.Mutex
	rng *rand.Rand
}

// NewLatencyHandler wraps inner, adding base (LatencyConstant) or base plus
// a uniform random amount up to jitter (LatencyJitter) of delay before
// every HandleRequest call reaches inner.
func NewLatencyHandler(inner request.Handler, base, jitter time.Duration, model LatencyModel) *LatencyHandler {
	return &LatencyHandler{
		inner:  inner,
		base:   base,
		jitter: jitter,
		model:  model,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// HandleRequest implements request.Handler: it sleeps for the configured
// latency, then delegates to the wrapped Handler.
func (h *LatencyHandler) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	if d := h.computeLatency(); d > 0 {
		time.Sleep(d)
	}
	return h.inner.HandleRequest(requester, req)
}

// computeLatency returns this call's simulated delay per the configured
// LatencyModel.
func (h *LatencyHandler) computeLatency() time.Duration {
	if h.model == LatencyJitter && h.jitter > 0 {
		h.mu.Lock()
		extra := time.Duration(h.rng.Int63n(int64(h.jitter) + 1))
		h.mu.Unlock()
		return h.base + extra
	}
	return h.base
}
