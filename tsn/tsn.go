// Package tsn provides an IEEE 802.1Qbv/802.1Qav-aware wrapper around this
// repo's udp.Controller for the OPEN Alliance TC18 Remote Control Protocol
// (RCP), as described by the "OPEN Alliance TC18 Remote Control Protocol
// Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 54 (v0.67.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, TSN scheduling is "a layer-2 QoS mechanism
// that is genuinely complementary to (not in conflict with) real IEEE 1722
// delivery," so this package keeps its SO_PRIORITY socket-option wiring
// (sockprio_linux.go/sockprio_other.go, both unchanged — they only ever
// depended on udp.Controller.RawConn, never on the retired framing) and
// only replaces what it wraps and what it derives PCP from.
//
// TSN (Time-Sensitive Networking) is a set of IEEE 802.1 amendments enabling
// deterministic, time-bounded Ethernet delivery. This package:
//
//   - Wraps udp.Controller, applying an IEEE 802.1p Priority Code Point
//     (PCP) socket priority to every request it sends
//   - Maps request.Kind.Priority()'s fixed cross-type execution-priority
//     rank (ROADMAP.md Milestone 49) to a PCP value, in place of the
//     retired rcp.Priority enum — see PCPMap
//   - Sets SO_PRIORITY on the UDP socket so the OS traffic shaper applies
//     the correct egress queue (requires Linux kernel >= 4.15 for full
//     802.1Qbv support)
//   - Exposes Config for per-controller VLAN/cycle-time bookkeeping
//
// Because actual 802.1Qbv gate scheduling requires NIC + kernel TSN support,
// this package operates on a best-effort basis on standard hardware and
// provides the full TSN API for use when appropriate hardware is available.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// request.Kind.Priority() ranks seven priority classes (0, highest, through
// 6, lowest — see request/kind.go); IEEE 802.1p PCP has eight levels (0-7).
// DefaultPCPMap's specific rank-to-PCP mapping is this implementation's own
// reasoned, self-consistent choice — the same open-item posture the retired
// package's own DefaultPCPMap already carried for its three-level
// rcp.Priority mapping — not a verified transcription of a published
// automotive PCP assignment table for this specific priority scheme.
package tsn

//fusa:req REQ-TSN-001
//fusa:req REQ-TSN-002
//fusa:req REQ-TSN-003
//fusa:req REQ-TSN-004
//fusa:req REQ-TSN-005
//fusa:req REQ-TSN-006

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/request"
	rcpudp "github.com/SoundMatt/go-RCP/v9/udp"
)

// PCPMap maps each request.Kind.Priority() rank (0-6) to an IEEE 802.1p PCP
// (Priority Code Point) value 0-7. PCP 7 is highest priority.
type PCPMap [7]uint8

// DefaultPCPMap returns this package's default rank-to-PCP mapping: rank 0
// (cancellation) at PCP 7 down to rank 6 (plain, the unconditional
// baseline) at PCP 1, leaving PCP 0 (best-effort/background) unused by any
// RCP request this package sends.
func DefaultPCPMap() PCPMap {
	return PCPMap{7, 6, 5, 4, 3, 2, 1}
}

// PCPFor returns the PCP value for k's priority rank, clamped to this map's
// bounds for a k this package's request package version does not define
// (forward-compatibility guard, not an expected runtime path).
func (m PCPMap) PCPFor(k request.Kind) uint8 {
	rank := k.Priority()
	if rank < 0 {
		rank = 0
	}
	if rank >= len(m) {
		rank = len(m) - 1
	}
	return m[rank]
}

// Config holds per-Controller TSN parameters.
type Config struct {
	// VLAN identifies the IEEE 802.1Q VLAN used for RCP traffic. 0 =
	// untagged (default).
	VLAN uint16

	// PCPMap maps request.Kind.Priority() rank to IEEE 802.1p PCP for
	// egress traffic shaping.
	PCPMap PCPMap

	// CycleNs is the TSN scheduling cycle time in nanoseconds (e.g. 500_000
	// for 500 us). 0 = best-effort (no cycle constraint).
	CycleNs uint32
}

// DefaultConfig returns a reasonable automotive TSN configuration.
func DefaultConfig() Config {
	return Config{
		VLAN:    100,
		PCPMap:  DefaultPCPMap(),
		CycleNs: 500_000, // 500 us (2 kHz scheduling cycle)
	}
}

// Controller wraps a udp.Controller, applying TSN priority metadata derived
// from Config to every request it sends.
type Controller struct {
	inner  *rcpudp.Controller
	cfg    Config
	closed bool
	mu     sync.Mutex
}

// NewController dials a TSN-aware Controller presenting streamID at
// serverAddr.
func NewController(streamID avtp.StreamID, serverAddr string, cfg Config) (*Controller, error) {
	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, fmt.Errorf("rcp/tsn: resolve %s: %w", serverAddr, err)
	}
	inner, err := rcpudp.NewController(streamID, addr)
	if err != nil {
		return nil, err
	}
	return &Controller{inner: inner, cfg: cfg}, nil
}

// StreamID returns this Controller's own avtp.StreamID identity.
func (c *Controller) StreamID() avtp.StreamID { return c.inner.StreamID() }

// Config returns the Config for this controller.
func (c *Controller) Config() Config { return c.cfg }

// PCPFor returns the PCP value TSN would apply to a request of Kind k.
func (c *Controller) PCPFor(k request.Kind) uint8 { return c.cfg.PCPMap.PCPFor(k) }

// Request applies the SO_PRIORITY socket option derived from kind's
// priority rank (on supported platforms), then dispatches via the inner
// udp.Controller.
func (c *Controller) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte, kind request.Kind) (acf.Message, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return acf.Message{}, fmt.Errorf("rcp/tsn: stream %s: %w", c.StreamID(), rcpudp.ErrClosed)
	}
	pcp := c.cfg.PCPMap.PCPFor(kind)
	c.mu.Unlock()

	setSocketPriority(c.inner, pcp)

	return c.inner.Request(ctx, addr, control, body)
}

// Read is Request with acf.FlagRead set, no body, and request.KindPlain.
func (c *Controller) Read(ctx context.Context, addr avtp.ByteBusID) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagRead, nil, request.KindPlain)
}

// Write is Request with acf.FlagWrite set and request.KindPlain.
func (c *Controller) Write(ctx context.Context, addr avtp.ByteBusID, body []byte) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagWrite, body, request.KindPlain)
}

// Close closes the inner udp.Controller.
func (c *Controller) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return c.inner.Close()
}

// Registry is a caller-keyed collection of TSN-aware Controllers, mirroring
// udp.Registry's own re-keying rationale (see udp/registry.go).
type Registry struct {
	mu     sync.RWMutex
	ctrls  map[string]*Controller
	closed bool
}

// NewRegistry returns an empty TSN Registry.
func NewRegistry() *Registry {
	return &Registry{ctrls: make(map[string]*Controller)}
}

// Dial creates and registers a TSN Controller under key.
func (r *Registry) Dial(key string, streamID avtp.StreamID, serverAddr string, cfg Config) (*Controller, error) {
	ctrl, err := NewController(streamID, serverAddr, cfg)
	if err != nil {
		return nil, err
	}
	if err := r.Register(key, ctrl); err != nil {
		_ = ctrl.Close()
		return nil, err
	}
	return ctrl, nil
}

// Register adds ctrl under key.
func (r *Registry) Register(key string, ctrl *Controller) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("rcp/tsn: registry: %w", rcpudp.ErrClosed)
	}
	if _, ok := r.ctrls[key]; ok {
		return fmt.Errorf("rcp/tsn: registry key %s: %w", key, rcpudp.ErrAlreadyExists)
	}
	r.ctrls[key] = ctrl
	return nil
}

// Deregister closes and removes the Controller registered under key.
func (r *Registry) Deregister(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrl, ok := r.ctrls[key]
	if !ok {
		return fmt.Errorf("rcp/tsn: registry key %s: %w", key, rcpudp.ErrNotFound)
	}
	delete(r.ctrls, key)
	return ctrl.Close()
}

// Lookup returns the Controller registered under key.
func (r *Registry) Lookup(key string) (*Controller, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, fmt.Errorf("rcp/tsn: registry: %w", rcpudp.ErrClosed)
	}
	ctrl, ok := r.ctrls[key]
	if !ok {
		return nil, fmt.Errorf("rcp/tsn: registry key %s: %w", key, rcpudp.ErrNotFound)
	}
	return ctrl, nil
}

// Controllers returns every currently registered Controller.
func (r *Registry) Controllers() []*Controller {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Controller, 0, len(r.ctrls))
	for _, c := range r.ctrls {
		out = append(out, c)
	}
	return out
}

// Close closes every registered Controller.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	var last error
	for key, ctrl := range r.ctrls {
		if err := ctrl.Close(); err != nil {
			last = err
		}
		delete(r.ctrls, key)
	}
	return last
}
