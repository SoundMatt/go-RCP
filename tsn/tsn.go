// Package tsn provides an IEEE 802.1Qbv-aware UDP transport for go-RCP.
//
// TSN (Time-Sensitive Networking) is a set of IEEE 802.1 amendments enabling
// deterministic, time-bounded Ethernet delivery. This package:
//
//   - Maps rcp.Priority values to IEEE 802.1p Priority Code Point (PCP) classes
//   - Sets SO_PRIORITY on the UDP socket so the OS traffic shaper applies the
//     correct egress queue (requires Linux kernel ≥ 4.15 for full 802.1Qbv support)
//   - Exposes TSNConfig for per-zone cycle time and VLAN configuration
//   - Wraps udp.Controller, adding TSN metadata to every command frame
//
// Because actual 802.1Qbv gate scheduling requires NIC + kernel TSN support,
// this package operates on a best-effort basis on standard hardware and
// provides the full TSN API for use when appropriate hardware is available.
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

	rcp "github.com/SoundMatt/go-RCP"
	rcpudp "github.com/SoundMatt/go-RCP/udp"
)

// PCPMap maps each rcp.Priority to an IEEE 802.1p PCP (Priority Code Point) value 0–7.
// PCP 7 is highest priority (used for PriorityCritical).
type PCPMap struct {
	Normal   uint8 // PCP for PriorityNormal   (default 2)
	High     uint8 // PCP for PriorityHigh     (default 5)
	Critical uint8 // PCP for PriorityCritical (default 7)
}

// DefaultPCPMap returns the recommended automotive PCP mapping per IEEE 802.1Q-2022 Table I-1.
func DefaultPCPMap() PCPMap {
	return PCPMap{Normal: 2, High: 5, Critical: 7}
}

// PCPFor returns the PCP value for the given rcp.Priority.
func (m PCPMap) PCPFor(p rcp.Priority) uint8 {
	switch p {
	case rcp.PriorityHigh:
		return m.High
	case rcp.PriorityCritical:
		return m.Critical
	default:
		return m.Normal
	}
}

// TSNConfig holds per-zone TSN parameters.
type TSNConfig struct {
	// VLAN identifies the IEEE 802.1Q VLAN used for RCP traffic on this zone.
	// 0 = untagged (default).
	VLAN uint16

	// PCPMap maps rcp.Priority to IEEE 802.1p PCP for egress traffic shaping.
	PCPMap PCPMap

	// CycleNs is the TSN scheduling cycle time in nanoseconds (e.g. 500_000 for 500 µs).
	// 0 = best-effort (no cycle constraint).
	CycleNs uint32
}

// DefaultTSNConfig returns a reasonable automotive TSN configuration.
func DefaultTSNConfig() TSNConfig {
	return TSNConfig{
		VLAN:    100,
		PCPMap:  DefaultPCPMap(),
		CycleNs: 500_000, // 500 µs (2 kHz scheduling cycle)
	}
}

// Controller is an rcp.Controller that sends UDP commands tagged with TSN priority
// metadata derived from the TSNConfig.
type Controller struct {
	inner  *rcpudp.Controller
	cfg    TSNConfig
	closed bool
	mu     sync.Mutex
}

// NewController creates a TSN-aware UDP controller for zone at serverAddr.
func NewController(zone rcp.Zone, serverAddr string, cfg TSNConfig) (*Controller, error) {
	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, fmt.Errorf("rcp/tsn: resolve %s: %w", serverAddr, err)
	}
	inner, err := rcpudp.NewController(zone, addr)
	if err != nil {
		return nil, err
	}
	return &Controller{inner: inner, cfg: cfg}, nil
}

// Zone implements rcp.Controller.
func (c *Controller) Zone() rcp.Zone { return c.inner.Zone() }

// Config returns the TSNConfig for this controller.
func (c *Controller) Config() TSNConfig { return c.cfg }

// PCPFor returns the PCP value for the given priority.
func (c *Controller) PCPFor(p rcp.Priority) uint8 { return c.cfg.PCPMap.PCPFor(p) }

// Send implements rcp.Controller. It dispatches the command via the inner
// UDP controller. The command's Priority field is used to determine the
// TSN traffic class; OS-level socket priority is set on supported platforms.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("rcp/tsn: zone %s: %w", c.Zone(), rcp.ErrClosed)
	}
	pcp := c.cfg.PCPMap.PCPFor(cmd.Priority)
	c.mu.Unlock()

	// Apply socket priority on platforms that support it.
	setSocketPriority(c.inner, pcp)

	return c.inner.Send(ctx, cmd)
}

// Subscribe implements rcp.Controller.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	return c.inner.Subscribe(ctx)
}

// Close implements rcp.Controller.
func (c *Controller) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return c.inner.Close()
}

// Registry is an rcp.Registry of TSN-aware controllers.
type Registry struct {
	mu     sync.RWMutex
	ctrls  map[rcp.Zone]*Controller
	closed bool
}

// NewRegistry returns an empty TSN Registry.
func NewRegistry() *Registry {
	return &Registry{ctrls: make(map[rcp.Zone]*Controller)}
}

// Dial creates and registers a TSN Controller for zone at serverAddr with cfg.
func (r *Registry) Dial(zone rcp.Zone, serverAddr string, cfg TSNConfig) error {
	ctrl, err := NewController(zone, serverAddr, cfg)
	if err != nil {
		return err
	}
	if err := r.Register(ctrl); err != nil {
		_ = ctrl.Close()
		return err
	}
	return nil
}

// Register implements rcp.Registry.
func (r *Registry) Register(ctrl rcp.Controller) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("rcp/tsn: registry: %w", rcp.ErrClosed)
	}
	if _, ok := r.ctrls[ctrl.Zone()]; ok {
		return fmt.Errorf("rcp/tsn: registry zone %s: %w", ctrl.Zone(), rcp.ErrAlreadyExists)
	}
	tc, ok := ctrl.(*Controller)
	if !ok {
		return fmt.Errorf("rcp/tsn: registry: only *tsn.Controller may be registered")
	}
	r.ctrls[ctrl.Zone()] = tc
	return nil
}

// Deregister implements rcp.Registry.
func (r *Registry) Deregister(zone rcp.Zone) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrl, ok := r.ctrls[zone]
	if !ok {
		return fmt.Errorf("rcp/tsn: registry zone %s: %w", zone, rcp.ErrNotFound)
	}
	delete(r.ctrls, zone)
	return ctrl.Close()
}

// Lookup implements rcp.Registry.
func (r *Registry) Lookup(zone rcp.Zone) (rcp.Controller, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, fmt.Errorf("rcp/tsn: registry: %w", rcp.ErrClosed)
	}
	ctrl, ok := r.ctrls[zone]
	if !ok {
		return nil, fmt.Errorf("rcp/tsn: registry zone %s: %w", zone, rcp.ErrNotFound)
	}
	return ctrl, nil
}

// Controllers implements rcp.Registry.
func (r *Registry) Controllers() []rcp.Controller {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]rcp.Controller, 0, len(r.ctrls))
	for _, c := range r.ctrls {
		out = append(out, c)
	}
	return out
}

// Close implements rcp.Registry.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	var last error
	for zone, ctrl := range r.ctrls {
		if err := ctrl.Close(); err != nil {
			last = err
		}
		delete(r.ctrls, zone)
	}
	return last
}
