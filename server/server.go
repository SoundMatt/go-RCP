package server

import (
	"sync"
	"time"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/discovery"
	"github.com/SoundMatt/go-RCP/lifecycle"
	"github.com/SoundMatt/go-RCP/regmap"
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
// or an explicit grant — see regmap.AccessController) and is rejected once
// lifecycle.StateFullyConfigured is reached, regardless of requester.
func (s *Server) WriteFunctional(requester avtp.StreamID, addr avtp.ByteBusID, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.access.CanAccess(requester, addr) {
		return regmap.ErrAccessDenied
	}
	if s.state == lifecycle.StateFullyConfigured {
		return regmap.ErrRegisterLocked
	}
	ep, ok := s.rmap.Endpoint(addr)
	if !ok {
		return regmap.ErrUnknownEndpoint
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
// never change (regmap.ErrGeneralBlockReadOnly), the pin-mapping table may
// not change once it is no longer lifecycle.StateUnconfigured
// (regmap.ErrRegisterLocked), and no part of the map may change at all once
// lifecycle.StateFullyConfigured is reached (regmap.ErrRegisterLocked). The
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

	if s.state == lifecycle.StateFullyConfigured {
		return regmap.ErrRegisterLocked
	}
	if s.state != lifecycle.StateUnconfigured {
		if !incoming.PinMap.Equal(&s.rmap.PinMap) {
			return regmap.ErrRegisterLocked
		}
		if !regmap.SameEndpointGenerics(incoming, s.rmap) {
			return regmap.ErrRegisterLocked
		}
	}

	s.rmap = incoming
	return nil
}
