package formal

import (
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/powerstate"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/wakeup"
)

// PowerStateOf snapshots ep's observable power state, together with drv's
// own retransmission bookkeeping, as a formal.State: the current
// wakeup.PowerState (by name), the most recent cold/hot-start
// determination (by name), and the number of wake-handshake repeats drv
// has pulled from ep but not yet transmitted.
func PowerStateOf(ep *wakeup.Endpoint, drv *powerstate.Driver) State {
	return State{
		"power":       ep.State().String(),
		"start_kind":  ep.LastStartKind().String(),
		"drv_pending": drv.Pending(),
	}
}

// PowerAction is one step a caller drives while building a PowerTrace: a
// requester-issued PowerState write against ep (see NewWakeCycleTrace's
// writeTo helper), or a powerstate.Driver.Pump call pacing out one queued
// wake-handshake repeat. Its error return is intentionally not propagated
// by PowerTrace, the same "the trace itself is what's under test, including
// rejected steps" posture LifecycleTrace documents.
type PowerAction func(ep *wakeup.Endpoint, drv *powerstate.Driver) error

// PowerTrace drives ep/drv through actions in order, recording
// PowerStateOf both before the first action and after every action.
func PowerTrace(ep *wakeup.Endpoint, drv *powerstate.Driver, actions []PowerAction) []State {
	trace := make([]State, 0, len(actions)+1)
	trace = append(trace, PowerStateOf(ep, drv))
	for _, action := range actions {
		_ = action(ep, drv)
		trace = append(trace, PowerStateOf(ep, drv))
	}
	return trace
}

// PowerInvariants returns the temporal properties this package verifies
// against any PowerTrace: wakeup.PowerUnpowered is never Endpoint's own
// observed current state (wakeup/doc.go's Scope section: a server that is
// actually unpowered cannot be running the code that would report that as
// its own state, so this is a genuine safety invariant, not merely an
// implementation detail); the wake-handshake retransmission queue
// eventually drains to zero once queued (powerstate's whole reason to
// exist — see powerstate/doc.go); and a Sleep→Normal wake eventually
// produces a cold/hot-start determination other than wakeup.StartUnknown.
func PowerInvariants() []Invariant {
	return []Invariant{
		{
			Name: "power state is never Unpowered",
			Check: Always(func(s State) bool {
				power, _ := s["power"].(string) //nolint:errcheck
				return power != wakeup.PowerUnpowered.String()
			}),
		},
		{
			Name: "wake-handshake retransmission queue eventually drains",
			Check: Eventually(func(s State) bool {
				pending, _ := s["drv_pending"].(int) //nolint:errcheck
				return pending == 0
			}),
		},
		{
			Name: "a wake eventually determines a cold/hot-start kind",
			Check: Eventually(func(s State) bool {
				kind, _ := s["start_kind"].(string) //nolint:errcheck
				return kind != wakeup.StartUnknown.String()
			}),
		},
	}
}

// NewWakeCycleTrace declares and configures a Wakeup endpoint at addr on a
// fresh server.Server (root claimed, requester for every action), wires a
// powerstate.Driver over it recording every transmitted WakeHandshake into
// sent, and returns the endpoint/driver pair plus the action sequence
// StandBy → Normal → Sleep → Normal (the one transition powerstate exists
// to pace, per its own doc comment) followed by enough Driver.Pump calls to
// drain and pace every queued repeat of cfg.WakeHandshakeRepeatCount.
// Passing the returned actions to PowerTrace produces a trace that
// satisfies every PowerInvariants property.
func NewWakeCycleTrace(root avtp.StreamID, addr avtp.ByteBusID) (*wakeup.Endpoint, *powerstate.Driver, []PowerAction, *[]wakeup.WakeHandshake, error) {
	srv := server.NewServer()
	if err := srv.ClaimRoot(root); err != nil {
		return nil, nil, nil, nil, err
	}
	if err := srv.AddEndpoint(root, addr, wakeup.EndpointType); err != nil {
		return nil, nil, nil, nil, err
	}
	ep := wakeup.NewEndpoint(srv, addr)
	cfg := wakeup.Config{Enabled: true, WakeHandshakeIntervalMillis: 10, WakeHandshakeRepeatCount: 3}
	if err := ep.Configure(root, cfg); err != nil {
		return nil, nil, nil, nil, err
	}

	sent := make([]wakeup.WakeHandshake, 0, cfg.WakeHandshakeRepeatCount)
	drv := powerstate.NewDriver(ep, root, func(_ avtp.StreamID, h wakeup.WakeHandshake) error {
		sent = append(sent, h)
		return nil
	})

	writeTo := func(target wakeup.PowerState) PowerAction {
		return func(e *wakeup.Endpoint, _ *powerstate.Driver) error {
			_, err := e.HandleRequest(root, acf.Message{
				ByteBusID: addr,
				Control:   acf.FlagWrite,
				Body:      wakeup.EncodePowerStateRequest(target),
			})
			return err
		}
	}
	pump := func(_ *wakeup.Endpoint, d *powerstate.Driver) error {
		_, err := d.Pump()
		return err
	}

	actions := []PowerAction{
		writeTo(wakeup.PowerStandBy),
		writeTo(wakeup.PowerNormal),
		writeTo(wakeup.PowerSleep),
		writeTo(wakeup.PowerNormal), // the wake: queues the change event plus 3 handshakes
		pump, pump, pump, pump,      // drains the change event and paces all 3 handshakes
	}
	return ep, drv, actions, &sent, nil
}
