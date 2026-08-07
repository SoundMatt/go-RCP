package server

import (
	"sync"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/discovery"
	"github.com/SoundMatt/go-RCP/v9/lifecycle"
	"github.com/SoundMatt/go-RCP/v9/regmap"
)

// Server is a single RC Server: its lifecycle state, its register map, and
// the EP0 access-control state (root-client claim and per-stream grants)
// that gates every read/write against that map. All exported methods are
// safe for concurrent use.
//
// Server also carries the Milestone 46 (Discovery) configuration-claim
// state (see discovery.go): a timeout-releasable reservation of
// configuration rights that coexists with, but is not the same mechanism
// as, regmap.AccessController's own permanent root-client claim.
//
// Server is this package's composition root: it holds a
// lifecycle.LifecycleState, a *regmap.RegisterMap, a *regmap.AccessController,
// and a discovery.Claim, and owns every transition guard and access-control
// decision that spans more than one of those — see doc.go's "A note on this
// package's shape" for why that composition, rather than each concern
// package doing it independently, is this package's whole reason to exist.
type Server struct {
	mu     sync.Mutex
	state  lifecycle.LifecycleState
	rmap   *regmap.RegisterMap
	access *regmap.AccessController

	now          func() time.Time // injectable for testing; defaults to time.Now
	claimTimeout time.Duration
	claim        discovery.Claim
}

// NewServer returns a Server in lifecycle.StateUnconfigured with an empty
// register map: no endpoints declared, an empty pin map, zero-value
// stream/queue configuration, and no root client claimed yet.
func NewServer() *Server {
	return NewServerWithClock(time.Now)
}

// NewServerWithClock is like NewServer but accepts a custom clock function,
// used in tests to avoid real-time sleeps when exercising the Discovery
// configuration-claim timeout (see ClaimConfiguration) — the same
// injectable-clock pattern ratelimit.NewControllerWithClock establishes.
func NewServerWithClock(now func() time.Time) *Server {
	return &Server{
		rmap:         regmap.NewRegisterMap(),
		access:       regmap.NewAccessController(),
		now:          now,
		claimTimeout: discovery.DefaultConfigurationClaimTimeout,
	}
}

// State returns the server's current lifecycle state.
func (s *Server) State() lifecycle.LifecycleState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// ClaimRoot establishes stream as the root client (see regmap.AccessController).
func (s *Server) ClaimRoot(stream avtp.StreamID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.access.ClaimRoot(stream)
}

// Grant gives a restricted stream access to endpoint ep.
func (s *Server) Grant(stream avtp.StreamID, ep avtp.ByteBusID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.access.Grant(stream, ep)
}

// AdvanceToHWLocked runs the Unconfigured→HWLocked transition guard: the
// current state must be lifecycle.StateUnconfigured, and the pin-mapping
// table must pass PinMap.Validate. On success the pin-mapping table (and
// every declared endpoint's GenericEndpointBlock) becomes permanently
// locked against further writes; on failure the state is unchanged.
func (s *Server) AdvanceToHWLocked() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != lifecycle.StateUnconfigured {
		return lifecycle.ErrLifecycleOutOfOrder
	}
	if err := s.rmap.PinMap.Validate(s.rmap); err != nil {
		return err
	}
	s.state = lifecycle.StateHWLocked
	return nil
}

// AdvanceToFullyConfigured runs the HWLocked→FullyConfigured transition
// guard: the current state must be lifecycle.StateHWLocked, every declared
// endpoint must have a non-empty functional configuration block, and the
// request-stream/queue configuration must pass QueueConfig.Validate. On
// success every register field becomes permanently locked, for every
// requester including the root client; on failure the state is unchanged.
func (s *Server) AdvanceToFullyConfigured() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != lifecycle.StateHWLocked {
		return lifecycle.ErrLifecycleOutOfOrder
	}
	for _, addr := range s.rmap.Addresses() {
		ep, _ := s.rmap.Endpoint(addr)
		if len(ep.Functional.Data) == 0 {
			return lifecycle.ErrFunctionalBlockIncomplete
		}
	}
	if err := s.rmap.Queues.Validate(); err != nil {
		return err
	}
	s.state = lifecycle.StateFullyConfigured
	return nil
}

// DemoteToUnconfigured runs the HWLocked→Unconfigured reverse-transition
// guard: the current state must be lifecycle.StateHWLocked, and requester
// must be either the root client or the stream currently holding the active
// Discovery-stream configuration claim (see ClaimConfiguration) — the same
// discovery-stream-or-root-client pairing that already governs functional
// configuration while HWLocked. On success the pin-mapping table and every
// declared endpoint's GenericEndpointBlock become writable again, exactly as
// they were in StateUnconfigured; declared endpoints, their pin assignments,
// and their functional blocks are left as-is rather than cleared, so a
// caller demoting the server to correct or extend the HW configuration does
// not lose unrelated state it never intended to touch. On failure the state
// is unchanged. There is no reverse transition out of
// lifecycle.StateFullyConfigured — see lifecycle.LifecycleState's doc
// comment.
func (s *Server) DemoteToUnconfigured(requester avtp.StreamID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != lifecycle.StateHWLocked {
		return lifecycle.ErrLifecycleOutOfOrder
	}
	if !s.access.IsRoot(requester) {
		now := s.now()
		if !s.claim.Active(now) || s.claim.Stream != requester {
			return lifecycle.ErrDemotionNotAuthorized
		}
	}
	s.state = lifecycle.StateUnconfigured
	return nil
}

// AddEndpoint declares a new endpoint at addr with the given type. It is a
// root-client-only, pre-HW-lock-only operation: declaring the server's
// endpoint topology is itself part of the hardware configuration the
// Unconfigured→HWLocked guard locks in.
func (s *Server) AddEndpoint(requester avtp.StreamID, addr avtp.ByteBusID, t regmap.EndpointType) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.IsRoot(requester) {
		return regmap.ErrNotRootClient
	}
	if s.state != lifecycle.StateUnconfigured {
		return regmap.ErrRegisterLocked
	}
	if addr == regmap.EP0 {
		return regmap.ErrReservedAddress
	}
	if s.rmap.HasEndpoint(addr) {
		return regmap.ErrDuplicateEndpoint
	}
	s.rmap.DeclareEndpoint(addr, t)
	return nil
}

// SetPinAssignment writes one entry of the HW pin-mapping table. It is a
// root-client-only, pre-HW-lock-only operation (see regmap.PinMap).
func (s *Server) SetPinAssignment(requester avtp.StreamID, a regmap.PinAssignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.IsRoot(requester) {
		return regmap.ErrNotRootClient
	}
	if s.state != lifecycle.StateUnconfigured {
		return regmap.ErrRegisterLocked
	}
	s.rmap.PinMap.Set(a)
	return nil
}

// SetStreamLimits writes the request-stream configuration table. Like a
// functional block, it stays writable through lifecycle.StateHWLocked and
// becomes permanently locked once lifecycle.StateFullyConfigured is
// reached.
func (s *Server) SetStreamLimits(requester avtp.StreamID, limits regmap.StreamLimits) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.IsRoot(requester) {
		return regmap.ErrNotRootClient
	}
	if s.state == lifecycle.StateFullyConfigured {
		return regmap.ErrRegisterLocked
	}
	s.rmap.Streams = limits
	return nil
}

// SetQueueConfig writes the response/acknowledge-queue configuration table.
// Like a functional block, it stays writable through
// lifecycle.StateHWLocked and becomes permanently locked once
// lifecycle.StateFullyConfigured is reached.
func (s *Server) SetQueueConfig(requester avtp.StreamID, cfg regmap.QueueConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.IsRoot(requester) {
		return regmap.ErrNotRootClient
	}
	if s.state == lifecycle.StateFullyConfigured {
		return regmap.ErrRegisterLocked
	}
	s.rmap.Queues = cfg
	return nil
}

// WriteFunctional writes an endpoint's functional (type-specific)
// configuration block. It requires requester to have access to addr (root,
// or an explicit grant — see regmap.AccessController). Unlike the
// server-wide/HW-pin configuration surfaces (SetPinAssignment,
// SetStreamLimits, SetQueueConfig), functional configuration stays
// writable, via addr's own registered stream(s) or via the root client,
// even once lifecycle.StateFullyConfigured is reached — see
// lifecycle.StateFullyConfigured's doc comment.
func (s *Server) WriteFunctional(requester avtp.StreamID, addr avtp.ByteBusID, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.CanAccess(requester, addr) {
		return regmap.ErrAccessDenied
	}
	ep, ok := s.rmap.Endpoint(addr)
	if !ok {
		return regmap.ErrUnknownEndpoint
	}
	ep.Functional.Data = append([]byte(nil), data...)
	return nil
}

// WriteFunctionalAt performs the offset-addressed functional-configuration
// write TC18 §12.7.1 defines for an evt[2:0] = 111b configuration request:
// data is written into addr's functional block starting at startAddr, a byte
// offset relative to that block's own base ("relative Register start address
// in EP_func", Figure 18), leaving every byte outside [startAddr,
// startAddr+len(data)) as it was. It is subject to exactly the same access
// rule as WriteFunctional.
//
// Per §12.7.1, "Any byte_msg_payload for which the length plus the
// start_address results in a value larger than the EP_LEN, is to be
// ignored": an overrunning write is silently discarded in full (no error, no
// truncation, no partial application), so this returns nil having changed
// nothing. EP_LEN is the addressed endpoint's current functional-block
// length — this call never grows the block, since the block's length is
// itself the endpoint's declared EP_LEN rather than a free-growing buffer. A
// write into an endpoint whose functional block has never been populated
// therefore lands in the ignore case, which is the correct outcome: there is
// no EP_func region to address yet.
//
// A zero-length data is a no-op that still performs the access and
// endpoint-existence checks, so a caller can use it to probe both.
func (s *Server) WriteFunctionalAt(requester avtp.StreamID, addr avtp.ByteBusID, startAddr uint16, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.CanAccess(requester, addr) {
		return regmap.ErrAccessDenied
	}
	ep, ok := s.rmap.Endpoint(addr)
	if !ok {
		return regmap.ErrUnknownEndpoint
	}
	if len(data) == 0 {
		return nil
	}
	end := int(startAddr) + len(data)
	if end > len(ep.Functional.Data) {
		return nil // §12.7.1: "is to be ignored"
	}
	copy(ep.Functional.Data[startAddr:end], data)
	return nil
}

// ApplyConfigRequest performs the TC18 §12.7.1 endpoint-configuration access
// an evt[2:0] = 111b request asks for (see acf.EVTActionConfigure), and
// returns the response body for it — nil for a write, the addressed EP_func
// slice for a read.
//
// This is the single shared implementation every endpoint-type package's
// HandleRequest delegates its configuration-change path to, rather than each
// package re-deriving §12.7.1 for itself. The two callbacks are the only
// type-specific part:
//
//   - encode returns the endpoint's current configuration rendered as its
//     EP_func block (typically the package's own EncodeConfig applied to its
//     cached Config). Its length is that endpoint's EP_LEN.
//   - adopt decodes, validates and adopts a patched EP_func block (typically
//     DecodeConfig + Config.Validate + storing the result). It must leave
//     the endpoint unchanged and return an error if the patched block is not
//     a valid configuration for this endpoint type.
//
// A write patches encode()'s block at the request's relative start address
// and, if adopt accepts it, persists it through WriteFunctional. Per
// §12.7.1, "Any byte_msg_payload for which the length plus the start_address
// results in a value larger than the EP_LEN, is to be ignored": such a write
// is silently discarded in full and answered with an ordinary empty success
// response. If persisting fails (e.g. the requester's grant was revoked
// between the two calls), the previously adopted block is restored before
// the error is returned, so the endpoint's in-memory configuration never
// diverges from the register map.
//
// A read returns req.ReadSizeOrSegment bytes of the EP_func block starting
// at the request's relative start address, clamped to the end of the block,
// or the whole remainder of the block when ReadSizeOrSegment is zero.
func (s *Server) ApplyConfigRequest(
	requester avtp.StreamID,
	addr avtp.ByteBusID,
	req acf.Message,
	encode func() []byte,
	adopt func([]byte) error,
) ([]byte, error) {
	start, data, err := acf.DecodeConfigRequestBody(req.Body)
	if err != nil {
		return nil, err
	}
	current := encode()

	if req.Control.Has(acf.FlagWrite) {
		end := int(start) + len(data)
		if end > len(current) {
			return nil, nil // §12.7.1: "is to be ignored"
		}
		patched := append([]byte(nil), current...)
		copy(patched[start:end], data)
		if err := adopt(patched); err != nil {
			return nil, err
		}
		if err := s.WriteFunctional(requester, addr, patched); err != nil {
			// Roll back to the block that was in effect before this
			// request; it decoded and validated once, so re-adopting it
			// cannot fail for any reason this one did not already.
			_ = adopt(current)
			return nil, err
		}
		return nil, nil
	}

	if int(start) >= len(current) {
		return nil, nil
	}
	tail := current[start:]
	if n := int(req.ReadSizeOrSegment); n > 0 && n < len(tail) {
		tail = tail[:n]
	}
	return append([]byte(nil), tail...), nil
}

// ReadEndpoint returns the encoded GenericEndpointBlock+FunctionalBlock for
// addr, subject to the same access rule as WriteFunctional.
func (s *Server) ReadEndpoint(requester avtp.StreamID, addr avtp.ByteBusID) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.CanAccess(requester, addr) {
		return nil, regmap.ErrAccessDenied
	}
	ep, ok := s.rmap.Endpoint(addr)
	if !ok {
		return nil, regmap.ErrUnknownEndpoint
	}
	return ep.Encode(), nil
}

// ReadEP0 returns the whole register map, encoded, for requester. Per this
// milestone's access model, EP0 is gated like any other address: requester
// must be root or have an explicit grant of EP0. See ReadDiscovery
// (discovery.go) for Milestone 46's separate, grant-independent and
// lifecycle-state-independent register-0 read.
func (s *Server) ReadEP0(requester avtp.StreamID) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.CanAccess(requester, regmap.EP0) {
		return nil, regmap.ErrAccessDenied
	}
	return regmap.EncodeRegisterMap(s.rmap), nil
}

// WriteEP0 replaces the whole register map from data, for requester. It is
// root-client-only. The incoming map is checked field-by-field against the
// current lock state before anything is applied: the general block may
// never change (regmap.ErrGeneralBlockReadOnly); the pin-mapping table and
// each endpoint's GenericEndpointBlock may not change once it is no longer
// lifecycle.StateUnconfigured (regmap.ErrRegisterLocked); and, once
// lifecycle.StateFullyConfigured is reached, the request-stream and
// response/acknowledge-queue configuration tables become locked too
// (regmap.ErrRegisterLocked) — but each endpoint's own functional
// (type-specific) configuration block remains writable via the root client
// through EP0 even in StateFullyConfigured, matching WriteFunctional. The
// write is all-or-nothing: on any rejection, the server's map is left
// exactly as it was.
func (s *Server) WriteEP0(requester avtp.StreamID, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.IsRoot(requester) {
		return regmap.ErrNotRootClient
	}

	incoming, err := regmap.DecodeRegisterMap(data)
	if err != nil {
		return err
	}
	if !regmap.SameGeneralIdentity(incoming.General, s.rmap.General) {
		return regmap.ErrGeneralBlockReadOnly
	}

	if s.state != lifecycle.StateUnconfigured {
		if !incoming.PinMap.Equal(&s.rmap.PinMap) {
			return regmap.ErrRegisterLocked
		}
		if !regmap.SameEndpointGenerics(incoming, s.rmap) {
			return regmap.ErrRegisterLocked
		}
	}
	if s.state == lifecycle.StateFullyConfigured {
		if incoming.Streams != s.rmap.Streams {
			return regmap.ErrRegisterLocked
		}
		if incoming.Queues != s.rmap.Queues {
			return regmap.ErrRegisterLocked
		}
	}

	s.rmap = incoming
	return nil
}
