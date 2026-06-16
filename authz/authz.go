// Package authz provides command-level access control for the RCP stack,
// implementing ISO 21434 SL-2 policy enforcement for automotive zonal architecture.
//
// A Policy maps (principal, zone, commandType) triples to a permit/deny decision.
// Principals are opaque string identifiers (e.g. ECU name, service account, role).
// A Controller wraps any rcp.Controller and enforces the policy on every Send;
// denied commands return ErrDenied without touching the inner controller.
//
// Wildcard entries (empty string principal, ZoneUnknown, or CmdType 0xFFFF)
// act as catch-all fallbacks, evaluated after exact matches.
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

	rcp "github.com/SoundMatt/go-RCP"
)

// ErrDenied is returned when a principal is not authorised to send a command.
var ErrDenied = errors.New("rcp/authz: command denied by policy")

// CmdTypeAny is the wildcard CommandType used in policy entries.
const CmdTypeAny rcp.CommandType = 0xFFFF

// Action specifies whether a matching policy entry permits or denies the command.
type Action uint8

const (
	Allow Action = iota + 1
	Deny
)

// Entry is a single policy rule.
// An empty Principal matches any caller. ZoneUnknown matches any zone.
// CmdTypeAny matches any CommandType. More-specific entries take precedence.
type Entry struct {
	Principal string
	Zone      rcp.Zone
	CmdType   rcp.CommandType
	Action    Action
}

// Policy holds an ordered set of access control entries.
// Evaluation stops at the first matching entry; if no entry matches, Deny is returned.
type Policy struct {
	mu      sync.RWMutex
	entries []Entry
}

// NewPolicy returns an empty policy (deny-all by default).
func NewPolicy() *Policy { return &Policy{} }

// Allow appends a permit entry.
func (p *Policy) Allow(principal string, zone rcp.Zone, cmdType rcp.CommandType) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries = append(p.entries, Entry{Principal: principal, Zone: zone, CmdType: cmdType, Action: Allow})
}

// Deny appends a deny entry.
func (p *Policy) Deny(principal string, zone rcp.Zone, cmdType rcp.CommandType) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries = append(p.entries, Entry{Principal: principal, Zone: zone, CmdType: cmdType, Action: Deny})
}

// SetEntries replaces all entries atomically.
func (p *Policy) SetEntries(entries []Entry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]Entry, len(entries))
	copy(cp, entries)
	p.entries = cp
}

// Evaluate returns true if the (principal, zone, cmdType) triple is permitted.
// Matching order: exact > wildcard-zone > wildcard-cmd > wildcard-both > default-deny.
func (p *Policy) Evaluate(principal string, zone rcp.Zone, cmdType rcp.CommandType) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, e := range p.entries {
		if !matchPrincipal(e.Principal, principal) {
			continue
		}
		if !matchZone(e.Zone, zone) {
			continue
		}
		if !matchCmdType(e.CmdType, cmdType) {
			continue
		}
		return e.Action == Allow
	}
	return false // default deny
}

func matchPrincipal(pattern, actual string) bool { return pattern == "" || pattern == actual }
func matchZone(pattern, actual rcp.Zone) bool {
	return pattern == rcp.ZoneUnknown || pattern == actual
}
func matchCmdType(pattern, actual rcp.CommandType) bool {
	return pattern == CmdTypeAny || pattern == actual
}

// Controller wraps any rcp.Controller and enforces a Policy on every Send.
// The caller's principal is supplied at construction time; use NewControllerFor
// when the principal is known statically, or attach it per-call via context (see WithPrincipal).
type Controller struct {
	inner     rcp.Controller
	policy    *Policy
	principal string
	closed    atomic.Bool
}

// NewController wraps inner with policy enforcement for the given principal.
func NewController(inner rcp.Controller, policy *Policy, principal string) *Controller {
	return &Controller{inner: inner, policy: policy, principal: principal}
}

// Send checks the policy for (principal, cmd.Zone, cmd.Type) before forwarding.
// Returns ErrDenied if the policy rejects the command.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/authz: zone %s: %w", c.inner.Zone(), rcp.ErrClosed)
	}
	principal := c.principal
	if p, ok := principalFromCtx(ctx); ok {
		principal = p
	}
	if !c.policy.Evaluate(principal, cmd.Zone, cmd.Type) {
		return nil, fmt.Errorf("rcp/authz: zone %s principal %q cmd %d: %w",
			cmd.Zone, principal, cmd.Type, ErrDenied)
	}
	return c.inner.Send(ctx, cmd)
}

// Zone delegates to the inner controller.
func (c *Controller) Zone() rcp.Zone { return c.inner.Zone() }

// Subscribe delegates to the inner controller.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	return c.inner.Subscribe(ctx)
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

// WithPrincipal returns a context that overrides the controller's static principal.
func WithPrincipal(ctx context.Context, principal string) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func principalFromCtx(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(principalKey{}).(string)
	return v, ok && v != ""
}
