//fusa:test REQ-ADC-004
//fusa:test REQ-ADC-005
//fusa:test REQ-ADC-006
//fusa:test REQ-ADC-007
//fusa:test REQ-ADC-008

package adc_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/adc"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
)

func writeReq() avtp.Message {
	return avtp.Message{Kind: avtp.KindShort, ByteBusID: avtp.ByteBusID(1), Control: avtp.FlagWrite}
}

func readReq() avtp.Message {
	return avtp.Message{Kind: avtp.KindShort, ByteBusID: avtp.ByteBusID(1), Control: avtp.FlagRead}
}

// sequenceTransport is a Transport test double that returns a fixed sequence
// of raw samples, one per call, so tests can drive predictable averaging.
type sequenceTransport struct {
	samples []uint16
	i       int
}

func (s *sequenceTransport) Sample() (uint16, error) {
	v := s.samples[s.i%len(s.samples)]
	s.i++
	return v, nil
}

// TestTrigger_SamplesAveragesAndCombines checks Trigger averages
// Config.SampleCount raw readings, combines under CombineReplace/
// CombineRollingAverage, and masks to Config.ResolutionBits (REQ-ADC-004).
func TestTrigger_SamplesAveragesAndCombines(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := adc.Config{Enabled: true, ResolutionBits: 8, SampleCount: 4, Combine: adc.CombineReplace, TriggerMode: adc.TriggerModeExternal}
	if cfgErr := ep.Configure(root, cfg); cfgErr != nil {
		t.Fatalf("Configure: %v", cfgErr)
	}
	ep.SetTransport(&sequenceTransport{samples: []uint16{100, 200, 300, 0xFFFF}}) // avg masked to 8 bits per-sample first

	got, err := ep.Trigger(root)
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	// Each raw sample is masked to 8 bits before averaging: 100&0xFF=100,
	// 200&0xFF=200, 300&0xFF=44, 0xFFFF&0xFF=255 -> avg = (100+200+44+255)/4 = 149.
	want := uint16(149)
	if got != want {
		t.Errorf("Trigger (CombineReplace) = %d, want %d", got, want)
	}

	// A second trigger with CombineRollingAverage blends with the previous
	// value.
	cfg.Combine = adc.CombineRollingAverage
	if cfgErr := ep.Configure(root, cfg); cfgErr != nil {
		t.Fatalf("Configure: %v", cfgErr)
	}
	ep.SetTransport(&sequenceTransport{samples: []uint16{50}})
	got, err = ep.Trigger(root)
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	wantRolling := uint16((149 + 50) / 2)
	if got != wantRolling {
		t.Errorf("Trigger (CombineRollingAverage) = %d, want %d", got, wantRolling)
	}
}

// TestTrigger_RejectsDisabledDefaultsToZeroTransport checks Trigger rejects
// an unconfigured channel, and defaults to an always-zero sample source with
// no Transport set (REQ-ADC-005).
func TestTrigger_RejectsDisabledDefaultsToZeroTransport(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if _, err := ep.Trigger(root); !errors.Is(err, adc.ErrChannelNotConfigured) {
		t.Errorf("Trigger(disabled) err = %v, want ErrChannelNotConfigured", err)
	}

	cfg := adc.Config{Enabled: true, ResolutionBits: 10, SampleCount: 3, Combine: adc.CombineReplace, TriggerMode: adc.TriggerModeExternal}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	got, err := ep.Trigger(root)
	if err != nil {
		t.Fatalf("Trigger(default transport): %v", err)
	}
	if got != 0 {
		t.Errorf("Trigger(default transport) = %d, want 0", got)
	}
}

// TestHandleRequest_RoutesReadWriteRejectsNeither checks HandleRequest
// dispatches Write to a manual trigger, and rejects a request with neither
// flag; wrong-endpoint and no-access requests are also rejected
// (REQ-ADC-006).
func TestHandleRequest_RoutesReadWriteRejectsNeither(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := adc.Config{Enabled: true, ResolutionBits: 8, SampleCount: 1, Combine: adc.CombineReplace, TriggerMode: adc.TriggerModeOnDemand}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	ep.SetTransport(&sequenceTransport{samples: []uint16{7}})

	resp, err := ep.HandleRequest(root, writeReq())
	if err != nil {
		t.Fatalf("HandleRequest(write): %v", err)
	}
	v, err := adc.DecodeValue(resp.Body)
	if err != nil || v != 7 {
		t.Errorf("DecodeValue(write response) = (%d, %v), want (7, nil)", v, err)
	}

	noFlags := avtp.Message{Kind: avtp.KindShort, ByteBusID: avtp.ByteBusID(1)}
	if _, err := ep.HandleRequest(root, noFlags); !errors.Is(err, adc.ErrRequestMustReadOrWrite) {
		t.Errorf("HandleRequest(no flags) err = %v, want ErrRequestMustReadOrWrite", err)
	}

	wrongAddr := writeReq()
	wrongAddr.ByteBusID = 2
	if _, err := ep.HandleRequest(root, wrongAddr); !errors.Is(err, adc.ErrWrongEndpoint) {
		t.Errorf("HandleRequest(wrong addr) err = %v, want ErrWrongEndpoint", err)
	}

	stranger := avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 9)
	if _, err := ep.HandleRequest(stranger, writeReq()); !errors.Is(err, server.ErrAccessDenied) {
		t.Errorf("HandleRequest(no grant) err = %v, want server.ErrAccessDenied", err)
	}
}

// TestHandleRequest_ReadBehaviorDependsOnTriggerMode checks a plain read
// samples fresh under TriggerModeOnDemand, but returns the cached value
// without sampling under TriggerModeExternal/TriggerModeSelf (REQ-ADC-007).
func TestHandleRequest_ReadBehaviorDependsOnTriggerMode(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := adc.Config{Enabled: true, ResolutionBits: 8, SampleCount: 1, Combine: adc.CombineReplace, TriggerMode: adc.TriggerModeOnDemand}
	if cfgErr := ep.Configure(root, cfg); cfgErr != nil {
		t.Fatalf("Configure: %v", cfgErr)
	}
	tr := &sequenceTransport{samples: []uint16{1, 2, 3}}
	ep.SetTransport(tr)

	resp, err := ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read, on-demand): %v", err)
	}
	v, _ := adc.DecodeValue(resp.Body)
	if v != 1 || tr.i != 1 {
		t.Errorf("read (on-demand) = %d (calls=%d), want 1 (calls=1)", v, tr.i)
	}

	cfg.TriggerMode = adc.TriggerModeExternal
	if cfgErr := ep.Configure(root, cfg); cfgErr != nil {
		t.Fatalf("Configure: %v", cfgErr)
	}
	resp, err = ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read, external, no fresh sample): %v", err)
	}
	v, _ = adc.DecodeValue(resp.Body)
	if v != 1 || tr.i != 1 {
		t.Errorf("read (external, cached) = %d (calls=%d), want 1 (calls=1, unchanged)", v, tr.i)
	}

	if _, triggerErr := ep.Trigger(root); triggerErr != nil {
		t.Fatalf("Trigger: %v", triggerErr)
	}
	resp, err = ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read, external, after externally-driven trigger): %v", err)
	}
	v, _ = adc.DecodeValue(resp.Body)
	if v != 2 {
		t.Errorf("read (external, after Trigger) = %d, want 2", v)
	}
}

// TestDrainTriggers_FIFOAndClears checks DrainTriggers returns queued
// TriggerMeasurementDone events in order and clears the queue (REQ-ADC-008).
func TestDrainTriggers_FIFOAndClears(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := adc.Config{Enabled: true, ResolutionBits: 8, SampleCount: 1, Combine: adc.CombineReplace, TriggerMode: adc.TriggerModeSelf}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	ep.SetTransport(&sequenceTransport{samples: []uint16{5, 9}})

	if _, err := ep.Trigger(root); err != nil {
		t.Fatalf("Trigger 1: %v", err)
	}
	if _, err := ep.Trigger(root); err != nil {
		t.Fatalf("Trigger 2: %v", err)
	}

	got := ep.DrainTriggers()
	if len(got) != 2 {
		t.Fatalf("DrainTriggers() = %+v, want 2 events", got)
	}
	if got[0].Value != 5 || got[1].Value != 9 {
		t.Errorf("DrainTriggers() values = [%d, %d], want [5, 9]", got[0].Value, got[1].Value)
	}
	if again := ep.DrainTriggers(); again != nil {
		t.Errorf("DrainTriggers() after drain = %+v, want nil", again)
	}
}
