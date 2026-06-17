//fusa:req REQ-ADAPT-001
//fusa:req REQ-ADAPT-002
//fusa:req REQ-ADAPT-003
//fusa:req REQ-ADAPT-004
//fusa:req REQ-ADAPT-005
//fusa:req REQ-ADAPT-006
//fusa:req REQ-ADAPT-007
//fusa:req REQ-ADAPT-008
//fusa:req REQ-MSG-003
//fusa:req REQ-MSG-004
//fusa:req REQ-MSG-005
//fusa:req REQ-MSG-006
//fusa:req REQ-MSG-007
//fusa:req REQ-MSG-008
//fusa:req REQ-MSG-009
//fusa:req REQ-MSG-010

package rcp

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	relay "github.com/SoundMatt/RELAY"
)

// Adapt wraps c as a relay.Caller so application code can use it
// protocol-agnostically. It is non-blocking and does not connect.
//
// The returned value also implements the optional relay interfaces
// HealthProvider, MetricsProvider, and Drainer (spec §9); application code
// may type-assert for them.
//
//fusa:req REQ-ADAPT-001
func Adapt(c Controller) relay.Caller {
	return &rcpAdapter{ctrl: c}
}

type rcpAdapter struct {
	ctrl Controller

	// Counters feeding MetricsProvider.Metrics() (spec §9).
	writeCount     atomic.Uint64
	deliverCount   atomic.Uint64
	dropCount      atomic.Uint64
	bytesWritten   atomic.Uint64
	bytesDelivered atomic.Uint64
	errorCount     atomic.Uint64

	// inFlight tracks Send/Call dispatches in progress, drained by
	// CloseWithDrain. closed feeds HealthProvider.Health().
	inFlight atomic.Int64
	closed   atomic.Bool
}

// Protocol returns relay.RCP.
//
//fusa:req REQ-ADAPT-002
func (a *rcpAdapter) Protocol() relay.Protocol { return relay.RCP }

// Send converts msg to a Command, dispatches it, and discards the Response.
//
//fusa:req REQ-ADAPT-003
func (a *rcpAdapter) Send(ctx context.Context, msg relay.Message) error {
	cmd, err := CommandFromMessage(msg)
	if err != nil {
		a.errorCount.Add(1)
		return err
	}
	a.inFlight.Add(1)
	defer a.inFlight.Add(-1)
	a.writeCount.Add(1)
	a.bytesWritten.Add(uint64(len(cmd.Payload)))
	if _, err = a.ctrl.Send(ctx, cmd); err != nil {
		a.errorCount.Add(1)
	}
	return err
}

// Call converts msg to a Command, dispatches it, and returns the Response
// as a relay.Message.
//
//fusa:req REQ-ADAPT-004
func (a *rcpAdapter) Call(ctx context.Context, req relay.Message) (relay.Message, error) {
	cmd, err := CommandFromMessage(req)
	if err != nil {
		a.errorCount.Add(1)
		return relay.Message{}, err
	}
	a.inFlight.Add(1)
	defer a.inFlight.Add(-1)
	a.writeCount.Add(1)
	a.bytesWritten.Add(uint64(len(cmd.Payload)))
	resp, err := a.ctrl.Send(ctx, cmd)
	if err != nil {
		a.errorCount.Add(1)
		return relay.Message{}, err
	}
	a.deliverCount.Add(1)
	a.bytesDelivered.Add(uint64(len(resp.Payload)))
	return ResponseToMessage(resp), nil
}

// Subscribe starts a goroutine that converts each *Status to relay.Message.
// The channel depth and back-pressure policy are taken from opts (default 64,
// DropNewest). The channel is closed when the underlying Controller closes.
//
//fusa:req REQ-ADAPT-005
//fusa:req REQ-ADAPT-006
//fusa:req REQ-ADAPT-007
func (a *rcpAdapter) Subscribe(opts ...relay.SubscriberOption) (<-chan relay.Message, error) {
	cfg := relay.ApplySubscriberOpts(opts)
	ch := make(chan relay.Message, cfg.ChanDepth(64))
	sub, err := a.ctrl.Subscribe(context.Background())
	if err != nil {
		return nil, err
	}
	go func() {
		defer close(ch)
		for s := range sub {
			msg := s.ToMessage()
			switch cfg.BackPressure {
			case relay.DropNewest:
				select {
				case ch <- msg:
					a.deliverCount.Add(1)
					a.bytesDelivered.Add(uint64(len(msg.Payload)))
				default:
					a.dropCount.Add(1)
				}
			case relay.DropOldest:
				select {
				case ch <- msg:
					a.deliverCount.Add(1)
					a.bytesDelivered.Add(uint64(len(msg.Payload)))
				default:
					<-ch
					a.dropCount.Add(1)
					ch <- msg
					a.deliverCount.Add(1)
					a.bytesDelivered.Add(uint64(len(msg.Payload)))
				}
			case relay.Block:
				ch <- msg
				a.deliverCount.Add(1)
				a.bytesDelivered.Add(uint64(len(msg.Payload)))
			}
		}
	}()
	return ch, nil
}

// Close closes the underlying Controller.
//
//fusa:req REQ-ADAPT-008
func (a *rcpAdapter) Close() error {
	a.closed.Store(true)
	return a.ctrl.Close()
}

// ToMessage converts s to a relay.Message per RELAY spec §15.7.5.
//
//fusa:req REQ-MSG-003
//fusa:req REQ-MSG-004
//fusa:req REQ-MSG-005
//fusa:req REQ-MSG-006
func (s *Status) ToMessage() relay.Message {
	return relay.Message{
		Protocol:  relay.RCP,
		ID:        s.Zone.String(),
		Payload:   s.Payload,
		Timestamp: time.Now(),
		Seq:       uint64(s.Seq),
		Meta: map[string]string{
			"rcp.healthy": strconv.FormatBool(s.Healthy),
		},
	}
}

// CommandFromMessage converts a relay.Message to a *Command.
// Returns ErrNotFound if m.ID is not a known zone string.
//
//fusa:req REQ-MSG-007
//fusa:req REQ-MSG-008
//fusa:req REQ-MSG-009
func CommandFromMessage(m relay.Message) (*Command, error) {
	zone, err := ZoneFromString(m.ID)
	if err != nil {
		return nil, err
	}
	cmd := &Command{
		Zone:    zone,
		Payload: m.Payload,
	}
	if v, ok := m.Meta["rcp.priority"]; ok {
		switch v {
		case "normal":
			cmd.Priority = PriorityNormal
		case "high":
			cmd.Priority = PriorityHigh
		case "critical":
			cmd.Priority = PriorityCritical
		}
	}
	if v, ok := m.Meta["rcp.cmd_type"]; ok {
		switch v {
		case "noop":
			cmd.Type = CmdNoop
		case "set":
			cmd.Type = CmdSet
		case "get":
			cmd.Type = CmdGet
		case "reset":
			cmd.Type = CmdReset
		case "watchdog":
			cmd.Type = CmdWatchdog
		case "sleep":
			cmd.Type = CmdSleep
		case "wake":
			cmd.Type = CmdWake
		}
	}
	return cmd, nil
}

// ResponseToMessage converts a *Response to a relay.Message.
//
//fusa:req REQ-MSG-010
func ResponseToMessage(r *Response) relay.Message {
	return relay.Message{
		Protocol:  relay.RCP,
		ID:        r.Zone.String(),
		Payload:   r.Payload,
		Timestamp: time.Now(),
		Meta: map[string]string{
			"rcp.status": strconv.Itoa(int(r.Status)),
		},
	}
}
