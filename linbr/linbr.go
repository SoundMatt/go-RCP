//fusa:req REQ-LIN-001
//fusa:req REQ-LIN-002
//fusa:req REQ-LIN-003
//fusa:req REQ-LIN-004
//fusa:req REQ-LIN-005
//fusa:req REQ-LIN-006
//fusa:req REQ-LIN-007
//fusa:req REQ-LIN-008

// Package linbr provides a LIN (Local Interconnect Network) bus simulation
// bridge for go-RCP.
//
// LIN is a low-cost single-wire serial bus used in automotive systems for
// body electronics (windows, mirrors, HVAC). This package simulates a LIN
// bus with a Master/Slave model: Master schedules frame headers; Slaves
// respond with data. Bridge maps an rcp.Controller to a LIN Slave so commands
// arrive via the simulated bus.
//
// Frame layout (8 bytes):
//   byte 0:    Protected ID (6-bit ID + 2-bit parity)
//   byte 1:    Data length (1–8)
//   bytes 2–9: Data payload (padded with 0xFF)
//   byte 10:   Checksum (classic: XOR of data bytes)
package linbr

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// ErrChecksumMismatch is returned when a received frame has an invalid checksum.
var ErrChecksumMismatch = errors.New("rcp/linbr: checksum mismatch")

// ErrFrameTooShort is returned when a frame is shorter than the minimum.
var ErrFrameTooShort = errors.New("rcp/linbr: frame too short")

const frameSize = 11

// Frame represents a LIN bus frame.
type Frame struct {
	ID       uint8
	DataLen  uint8
	Data     [8]byte
	Checksum uint8
}

// protectedID computes the LIN protected ID (6-bit id + 2 parity bits).
func protectedID(id uint8) uint8 {
	id &= 0x3F
	p0 := (id>>0 ^ id>>1 ^ id>>2 ^ id>>4) & 1
	p1 := ^(id>>1 ^ id>>3 ^ id>>4 ^ id>>5) & 1
	return id | (p0 << 6) | (p1 << 7)
}

// classicChecksum computes the XOR checksum of the data bytes.
func classicChecksum(data []byte) uint8 {
	var cs uint8
	for _, b := range data {
		cs ^= b
	}
	return cs
}

// EncodeFrame serialises f into an 11-byte slice.
func EncodeFrame(f Frame) []byte {
	buf := make([]byte, frameSize)
	buf[0] = protectedID(f.ID)
	n := f.DataLen
	if n > 8 {
		n = 8
	}
	buf[1] = n
	copy(buf[2:], f.Data[:n])
	for i := n; i < 8; i++ {
		buf[2+i] = 0xFF
	}
	buf[10] = classicChecksum(buf[2 : 2+n])
	return buf
}

// DecodeFrame parses an 11-byte LIN frame. Returns ErrChecksumMismatch on bad checksum.
func DecodeFrame(buf []byte) (Frame, error) {
	if len(buf) < frameSize {
		return Frame{}, ErrFrameTooShort
	}
	pid := buf[0]
	n := buf[1]
	if n > 8 {
		n = 8
	}
	want := classicChecksum(buf[2 : 2+n])
	got := buf[10]
	if want != got {
		return Frame{}, fmt.Errorf("%w: want 0x%02X got 0x%02X", ErrChecksumMismatch, want, got)
	}
	var f Frame
	f.ID = pid & 0x3F
	f.DataLen = n
	copy(f.Data[:], buf[2:2+n])
	f.Checksum = got
	return f, nil
}

// ─── Bus ─────────────────────────────────────────────────────────────────────

// Bus is an in-process LIN bus simulation.
// Frames written by the Master are delivered to all registered Slaves.
type Bus struct {
	mu     sync.RWMutex
	slaves map[uint8][]chan Frame
}

// NewBus returns an empty Bus.
func NewBus() *Bus { return &Bus{slaves: make(map[uint8][]chan Frame)} }

// Send delivers f to all Slaves registered for f.ID.
func (b *Bus) Send(f Frame) {
	b.mu.RLock()
	list := b.slaves[f.ID]
	b.mu.RUnlock()
	for _, ch := range list {
		select {
		case ch <- f:
		default:
		}
	}
}

func (b *Bus) registerSlave(id uint8, ch chan Frame) {
	b.mu.Lock()
	b.slaves[id] = append(b.slaves[id], ch)
	b.mu.Unlock()
}

func (b *Bus) deregisterSlave(id uint8, ch chan Frame) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.slaves[id]
	for i, c := range list {
		if c == ch {
			b.slaves[id] = append(list[:i], list[i+1:]...)
			return
		}
	}
}

// ─── Slave ────────────────────────────────────────────────────────────────────

// Slave listens for frames with a specific ID on the Bus.
type Slave struct {
	id  uint8
	bus *Bus
	ch  chan Frame
}

// NewSlave registers a Slave for id on bus.
func NewSlave(bus *Bus, id uint8) *Slave {
	ch := make(chan Frame, 32)
	bus.registerSlave(id, ch)
	return &Slave{id: id, bus: bus, ch: ch}
}

// Frames returns the channel of received frames.
func (s *Slave) Frames() <-chan Frame { return s.ch }

// Close deregisters the Slave from the bus.
func (s *Slave) Close() { s.bus.deregisterSlave(s.id, s.ch) }

// ─── Bridge ──────────────────────────────────────────────────────────────────

// Bridge maps a LIN Slave to an rcp.Controller.
// Each frame received by the Slave is decoded into an rcp.Command and dispatched.
// Frame wire format used by Bridge: Data[0]=Zone, Data[1]=CommandType, rest=Payload.
type Bridge struct {
	ctrl   rcp.Controller
	slave  *Slave
	closed atomic.Bool
	stop   chan struct{}
	wg     sync.WaitGroup
}

// NewBridge attaches ctrl to slave and starts a dispatch goroutine.
func NewBridge(ctrl rcp.Controller, slave *Slave) *Bridge {
	b := &Bridge{ctrl: ctrl, slave: slave, stop: make(chan struct{})}
	b.wg.Add(1)
	go b.run()
	return b
}

// Close stops the bridge goroutine. Idempotent.
func (b *Bridge) Close() {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}
	close(b.stop)
	b.wg.Wait()
}

func (b *Bridge) run() {
	defer b.wg.Done()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-b.stop; cancel() }()
	defer cancel()

	for {
		select {
		case <-b.stop:
			return
		case f, ok := <-b.slave.Frames():
			if !ok {
				return
			}
			if f.DataLen < 2 {
				continue
			}
			cmd := &rcp.Command{
				Zone:    rcp.Zone(f.Data[0]),
				Type:    rcp.CommandType(f.Data[1]),
				Payload: append([]byte(nil), f.Data[2:f.DataLen]...),
			}
			_, _ = b.ctrl.Send(ctx, cmd)
		}
	}
}
