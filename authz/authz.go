// Package authz provides a client-side, stream/endpoint-keyed access-control
// policy layer for the OPEN Alliance TC18 Remote Control Protocol (RCP), as
// described by the "OPEN Alliance TC18 Remote Control Protocol Specification
// v0.5.1_RC".
//
// This is ROADMAP.md Milestone 55 (v0.68.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, the specification already bakes a coarse
// access-control primitive into the server itself — regmap.AccessController's
// root-client/grant model (see regmap/access.go), fronted at the wire level
// by udp.EP0Handler — so this package is explicitly a complement to that
// server-side enforcement, not a duplicate of it. A caller wraps its own
// *udp.Controller in a Policy scoped to (principal, requester stream,
// target endpoint) so a locally misbehaving or misconfigured caller is
// rejected before a request ever reaches the wire, in addition to whatever
// the remote server itself would have rejected.
//
// The retired triple this package's Policy/Entry keyed on —
// (principal, rcp.Zone, rcp.CommandType) — has no equivalent in the new
// addressing model: there is no Zone, and no closed CommandType enum (every
// endpoint interprets its own request body). The natural re-keying is
// (principal, avtp.StreamID, avtp.ByteBusID): which caller identity, acting
// as which requester stream, may reach which endpoint.
package authz

//fusa:req REQ-AZ-001
//fusa:req REQ-AZ-002
//fusa:req REQ-AZ-003
//fusa:req REQ-AZ-004
//fusa:req REQ-AZ-005
//fusa:req REQ-AZ-006
//fusa:req REQ-AZ-007
//fusa:req REQ-AZ-008

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/udp"
)

// ErrDenied is returned when a principal is not authorised to reach an
// endpoint under the current Policy.
var ErrDenied = errors.New("rcp/authz: request denied by policy")

// EndpointAny is the wildcard avtp.ByteBusID used in policy entries. It is
// the largest value the type can hold, the same "reserved value doubles as
// wildcard" convention the retired package's CmdTypeAny (0xFFFF) already
// established.
const EndpointAny avtp.ByteBusID = 0xFF

// StreamAny is the wildcard avtp.StreamID used in policy entries: the IEEE
// 802 broadcast address (FF:FF:FF:FF:FF:FF), which is never a legitimate
// unicast sender StreamID, so it cannot collide with a real requester
// identity.
var StreamAny = avtp.StreamID{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}

// Action specifies whether a matching policy entry permits or denies the
// request.
type Action uint8

const (
	Allow Action = iota + 1
	Deny
)

// Entry is a single policy rule.
// An empty Principal matches any caller. StreamAny matches any requester
// stream. EndpointAny matches any endpoint. More-specific entries take
// precedence by evaluation order (see Policy.Evaluate).
type Entry struct {
	Principal string
	Stream    avtp.StreamID
	Endpoint  avtp.ByteBusID
	Action    Action
}

// Policy holds an ordered set of access control entries.
// Evaluation stops at the first matching entry; if no entry matches, Deny is
// returned.
type Policy struct {
	mu      sync.RWMutex
	entries []Entry
}

// NewPolicy returns an empty policy (deny-all by default).
func NewPolicy() *Policy { return &Policy{} }

// Allow appends a permit entry.
func (p *Policy) Allow(principal string, stream avtp.StreamID, endpoint avtp.ByteBusID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries = append(p.entries, Entry{Principal: principal, Stream: stream, Endpoint: endpoint, Action: Allow})
}

// Deny appends a deny entry.
func (p *Policy) Deny(principal string, stream avtp.StreamID, endpoint avtp.ByteBusID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries = append(p.entries, Entry{Principal: principal, Stream: stream, Endpoint: endpoint, Action: Deny})
}

// SetEntries replaces all entries atomically.
func (p *Policy) SetEntries(entries []Entry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]Entry, len(entries))
	copy(cp, entries)
	p.entries = cp
}

// Evaluate returns true if the (principal, stream, endpoint) triple is
// permitted. Matching order follows Entry's declared order; the first
// matching entry (exact or wildcard) decides the outcome.
func (p *Policy) Evaluate(principal string, stream avtp.StreamID, endpoint avtp.ByteBusID) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, e := range p.entries {
		if !matchPrincipal(e.Principal, principal) {
			continue
		}
		if !matchStream(e.Stream, stream) {
			continue
		}
		if !matchEndpoint(e.Endpoint, endpoint) {
			continue
		}
		return e.Action == Allow
	}
	return false // default deny
}

func matchPrincipal(pattern, actual string) bool { return pattern == "" || pattern == actual }
func matchStream(pattern, actual avtp.StreamID) bool {
	return pattern == StreamAny || pattern == actual
}
func matchEndpoint(pattern, actual avtp.ByteBusID) bool {
	return pattern == EndpointAny || pattern == actual
}

// Controller wraps a *udp.Controller and enforces a Policy on every Request,
// keyed by (principal, the wrapped Controller's own requester StreamID,
// the target endpoint). The caller's principal is supplied at construction
// time; use NewController when the principal is known statically, or attach
// it per-call via context (see WithPrincipal).
type Controller struct {
	inner     *udp.Controller
	policy    *Policy
	principal string
	closed    atomic.Bool
}

// NewController wraps inner with policy enforcement for the given
// principal.
func NewController(inner *udp.Controller, policy *Policy, principal string) *Controller {
	return &Controller{inner: inner, policy: policy, principal: principal}
}

// StreamID returns the inner controller's own avtp.StreamID identity.
func (c *Controller) StreamID() avtp.StreamID { return c.inner.StreamID() }

// Request checks the policy for (principal, StreamID, addr) before
// forwarding to the inner controller. Returns ErrDenied if the policy
// rejects the request.
func (c *Controller) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	if c.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/authz: stream %s: %w", c.StreamID(), udp.ErrClosed)
	}
	principal := c.principal
	if p, ok := principalFromCtx(ctx); ok {
		principal = p
	}
	if !c.policy.Evaluate(principal, c.StreamID(), addr) {
		return acf.Message{}, fmt.Errorf("rcp/authz: stream %s principal %q endpoint %d: %w",
			c.StreamID(), principal, addr, ErrDenied)
	}
	return c.inner.Request(ctx, addr, control, body)
}

// Read is Request with acf.FlagRead set and no body.
func (c *Controller) Read(ctx context.Context, addr avtp.ByteBusID) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagRead, nil)
}

// Write is Request with acf.FlagWrite set and the given body.
func (c *Controller) Write(ctx context.Context, addr avtp.ByteBusID, body []byte) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagWrite, body)
}

// Discover delegates directly to the inner controller without a policy
// check, mirroring regmap.AccessController's own deliberate exception for
// discovery: the specification's discovery mechanism is universal and
// grant-independent server-side (see regmap/access.go), so a client-side
// policy gate in front of it would only add a caller-local restriction with
// no corresponding server-side concept to complement.
func (c *Controller) Discover(ctx context.Context) ([]byte, error) {
	return c.inner.Discover(ctx)
}

// Close closes the inner controller. Safe to call multiple times.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.inner.Close()
}

// principalKey is the context key for per-call principal override.
type principalKey struct{}

// WithPrincipal returns a context that overrides the controller's static
// principal.
func WithPrincipal(ctx context.Context, principal string) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func principalFromCtx(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(principalKey{}).(string)
	return v, ok && v != ""
}
