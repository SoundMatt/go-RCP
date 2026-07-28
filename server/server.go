package server

import (
	"sync"
	"time"

	"github.com/SoundMatt/go-RCP/avtp"
)

// Server is a single RC Server: its lifecycle state, its register map, and
// the EP0 access-control state (root-client claim and per-stream grants)
// that gates every read/write against that map. All exported methods are
// safe for concurrent use.
//
// Server also carries the Milestone 46 (Discovery) configuration-claim
// state (see discovery.go): a timeout-releasable reservation of
// configuration rights that coexists with, but is not the same mechanism
// as, AccessController's own permanent root-client claim.
type Server struct {
	mu     sync.Mutex
	state  LifecycleState
	regmap *RegisterMap
	access *AccessController

	now          func() time.Time // injectable for testing; defaults to time.Now
	claimTimeout time.Duration
	claim        configurationClaim
}

// NewServer returns a Server in StateUnconfigured with an empty register
// map: no endpoints declared, an empty pin map, zero-value stream/queue
// configuration, and no root client claimed yet.
func NewServer() *Server {
	return NewServerWithClock(time.Now)
}

// NewServerWithClock is like NewServer but accepts a custom clock function,
// used in tests to avoid real-time sleeps when exercising the Discovery
// configuration-claim timeout (see ClaimConfiguration) — the same
// injectable-clock pattern ratelimit.NewControllerWithClock establishes.
func NewServerWithClock(now func() time.Time) *Server {
	return &Server{
		regmap:       NewRegisterMap(),
		access:       NewAccessController(),
		now:          now,
		claimTimeout: DefaultConfigurationClaimTimeout,
	}
}

// State returns the server's current lifecycle state.
func (s *Server) State() LifecycleState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// ClaimRoot establishes stream as the root client (see AccessController).
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
// current state must be StateUnconfigured, and the pin-mapping table must
// pass PinMap.Validate. On success the pin-mapping table (and every
// declared endpoint's GenericEndpointBlock) becomes permanently locked
// against further writes; on failure the state is unchanged.
func (s *Server) AdvanceToHWLocked() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateUnconfigured {
		return ErrLifecycleOutOfOrder
	}
	if err := s.regmap.PinMap.Validate(s.regmap); err != nil {
		return err
	}
	s.state = StateHWLocked
	return nil
}

// AdvanceToFullyConfigured runs the HWLocked→FullyConfigured transition
// guard: the current state must be StateHWLocked, every declared endpoint
// must have a non-empty functional configuration block, and the
// request-stream/queue configuration must pass QueueConfig.Validate. On
// success every register field becomes permanently locked, for every
// requester including the root client; on failure the state is unchanged.
func (s *Server) AdvanceToFullyConfigured() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateHWLocked {
		return ErrLifecycleOutOfOrder
	}
	for _, addr := range s.regmap.Addresses() {
		ep := s.regmap.endpoints[addr]
		if len(ep.Functional.Data) == 0 {
			return ErrFunctionalBlockIncomplete
		}
	}
	if err := s.regmap.Queues.Validate(); err != nil {
		return err
	}
	s.state = StateFullyConfigured
	return nil
}

// AddEndpoint declares a new endpoint at addr with the given type. It is a
// root-client-only, pre-HW-lock-only operation: declaring the server's
// endpoint topology is itself part of the hardware configuration the
// Unconfigured→HWLocked guard locks in.
func (s *Server) AddEndpoint(requester avtp.StreamID, addr avtp.ByteBusID, t EndpointType) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.IsRoot(requester) {
		return ErrNotRootClient
	}
	if s.state != StateUnconfigured {
		return ErrRegisterLocked
	}
	if addr == EP0 {
		return ErrReservedAddress
	}
	if _, exists := s.regmap.endpoints[addr]; exists {
		return ErrDuplicateEndpoint
	}
	s.regmap.endpoints[addr] = &EndpointRegisters{
		Generic: GenericEndpointBlock{Address: addr, Type: t, Enabled: true},
	}
	return nil
}

// SetPinAssignment writes one entry of the HW pin-mapping table. It is a
// root-client-only, pre-HW-lock-only operation (see PinMap).
func (s *Server) SetPinAssignment(requester avtp.StreamID, a PinAssignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.IsRoot(requester) {
		return ErrNotRootClient
	}
	if s.state != StateUnconfigured {
		return ErrRegisterLocked
	}
	s.regmap.PinMap.Set(a)
	return nil
}

// SetStreamLimits writes the request-stream configuration table. Like a
// functional block, it stays writable through StateHWLocked and becomes
// permanently locked once StateFullyConfigured is reached.
func (s *Server) SetStreamLimits(requester avtp.StreamID, limits StreamLimits) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.IsRoot(requester) {
		return ErrNotRootClient
	}
	if s.state == StateFullyConfigured {
		return ErrRegisterLocked
	}
	s.regmap.Streams = limits
	return nil
}

// SetQueueConfig writes the response/acknowledge-queue configuration table.
// Like a functional block, it stays writable through StateHWLocked and
// becomes permanently locked once StateFullyConfigured is reached.
func (s *Server) SetQueueConfig(requester avtp.StreamID, cfg QueueConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.IsRoot(requester) {
		return ErrNotRootClient
	}
	if s.state == StateFullyConfigured {
		return ErrRegisterLocked
	}
	s.regmap.Queues = cfg
	return nil
}

// WriteFunctional writes an endpoint's functional (type-specific)
// configuration block. It requires requester to have access to addr (root,
// or an explicit grant — see AccessController) and is rejected once
// StateFullyConfigured is reached, regardless of requester.
func (s *Server) WriteFunctional(requester avtp.StreamID, addr avtp.ByteBusID, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.CanAccess(requester, addr) {
		return ErrAccessDenied
	}
	if s.state == StateFullyConfigured {
		return ErrRegisterLocked
	}
	ep, ok := s.regmap.endpoints[addr]
	if !ok {
		return ErrUnknownEndpoint
	}
	ep.Functional.Data = append([]byte(nil), data...)
	return nil
}

// ReadEndpoint returns the encoded GenericEndpointBlock+FunctionalBlock for
// addr, subject to the same access rule as WriteFunctional.
func (s *Server) ReadEndpoint(requester avtp.StreamID, addr avtp.ByteBusID) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.CanAccess(requester, addr) {
		return nil, ErrAccessDenied
	}
	ep, ok := s.regmap.endpoints[addr]
	if !ok {
		return nil, ErrUnknownEndpoint
	}
	out := encodeGenericEndpointBlock(ep.Generic)
	out = append(out, encodeFunctionalBlock(ep.Functional)...)
	return out, nil
}

// ReadEP0 returns the whole register map, encoded, for requester. Per this
// milestone's access model, EP0 is gated like any other address: requester
// must be root or have an explicit grant of EP0. See ReadDiscovery
// (discovery.go) for Milestone 46's separate, grant-independent and
// lifecycle-state-independent register-0 read.
func (s *Server) ReadEP0(requester avtp.StreamID) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.CanAccess(requester, EP0) {
		return nil, ErrAccessDenied
	}
	return EncodeRegisterMap(s.regmap), nil
}

// WriteEP0 replaces the whole register map from data, for requester. It is
// root-client-only. The incoming map is checked field-by-field against the
// current lock state before anything is applied: the general block may
// never change (ErrGeneralBlockReadOnly), the pin-mapping table may not
// change once it is no longer StateUnconfigured (ErrRegisterLocked), and no
// part of the map may change at all once StateFullyConfigured is reached
// (ErrRegisterLocked). The write is all-or-nothing: on any rejection, the
// server's map is left exactly as it was.
func (s *Server) WriteEP0(requester avtp.StreamID, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.IsRoot(requester) {
		return ErrNotRootClient
	}

	incoming, err := DecodeRegisterMap(data)
	if err != nil {
		return err
	}
	if !sameGeneralIdentity(incoming.General, s.regmap.General) {
		return ErrGeneralBlockReadOnly
	}

	if s.state == StateFullyConfigured {
		return ErrRegisterLocked
	}
	if s.state != StateUnconfigured {
		if !incoming.PinMap.Equal(&s.regmap.PinMap) {
			return ErrRegisterLocked
		}
		if !sameEndpointGenerics(incoming, s.regmap) {
			return ErrRegisterLocked
		}
	}

	s.regmap = incoming
	return nil
}

// sameEndpointGenerics reports whether a and b declare exactly the same set
// of endpoint addresses with exactly the same GenericEndpointBlock at each
// one. Endpoint declaration (address/type/enabled) is locked in alongside
// the pin-mapping table once the server leaves StateUnconfigured, the same
// as AdvanceToHWLocked's doc comment describes; only each endpoint's
// FunctionalBlock, and the stream/queue tables, may still change.
func sameEndpointGenerics(a, b *RegisterMap) bool {
	if len(a.endpoints) != len(b.endpoints) {
		return false
	}
	for addr, ea := range a.endpoints {
		eb, ok := b.endpoints[addr]
		if !ok || ea.Generic != eb.Generic {
			return false
		}
	}
	return true
}
