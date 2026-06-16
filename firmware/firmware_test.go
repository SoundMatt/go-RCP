//fusa:test REQ-FW-001
//fusa:test REQ-FW-002
//fusa:test REQ-FW-003
//fusa:test REQ-FW-004
//fusa:test REQ-FW-005
//fusa:test REQ-FW-006
//fusa:test REQ-FW-007
//fusa:test REQ-FW-008

package firmware_test

import (
	"bytes"
	"context"
	"errors"
	"hash/crc32"
	"sync"
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/firmware"
)

// chunkRecorder is a fake rcp.Controller that records received chunks.
type chunkRecorder struct {
	mu       sync.Mutex
	chunks   []firmware.ChunkPayload
	failAt   int // -1 = never fail; >= 0 = fail at that chunk index
	callIdx  int
	verifyOK bool // whether to return OK on the verify call
}

func newRecorder(failAt int, verifyOK bool) *chunkRecorder {
	return &chunkRecorder{failAt: failAt, verifyOK: verifyOK}
}

func (r *chunkRecorder) Send(_ context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp, err := firmware.UnmarshalChunkPayload(cmd.Payload)
	if err != nil {
		return &rcp.Response{Status: rcp.StatusError}, nil
	}
	isVerify := cp.Offset == cp.TotalSize
	if !isVerify {
		if r.failAt >= 0 && r.callIdx == r.failAt {
			r.callIdx++
			return &rcp.Response{Status: rcp.StatusError}, nil
		}
		r.chunks = append(r.chunks, cp)
		r.callIdx++
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}, nil
	}
	// verify call
	if !r.verifyOK {
		return &rcp.Response{Status: rcp.StatusError}, nil
	}
	return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}, nil
}

func (r *chunkRecorder) Zone() rcp.Zone                                        { return rcp.ZoneFrontLeft }
func (r *chunkRecorder) Subscribe(_ context.Context) (<-chan *rcp.Status, error) { ch := make(chan *rcp.Status); return ch, nil }
func (r *chunkRecorder) Close() error                                           { return nil }

func (r *chunkRecorder) Assembled() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.chunks) == 0 {
		return nil
	}
	total := int(r.chunks[0].TotalSize)
	buf := make([]byte, total)
	for _, cp := range r.chunks {
		end := int(cp.Offset) + len(cp.Data)
		if end > total {
			end = total
		}
		copy(buf[cp.Offset:end], cp.Data)
	}
	return buf
}

// TestFirmware_SuccessfulUpdate delivers an image and verifies reassembly (REQ-FW-001, REQ-FW-002).
func TestFirmware_SuccessfulUpdate(t *testing.T) {
	image := bytes.Repeat([]byte{0xAB}, 1024)
	rec := newRecorder(-1, true)
	u := firmware.NewUpdater(rec, firmware.Config{ChunkSize: 256, Priority: rcp.PriorityHigh})

	if err := u.Update(context.Background(), image); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := rec.Assembled()
	if !bytes.Equal(got, image) {
		t.Errorf("reassembled image mismatch: got %d bytes", len(got))
	}
}

// TestFirmware_ChunkCount verifies correct number of chunks (REQ-FW-002).
func TestFirmware_ChunkCount(t *testing.T) {
	image := make([]byte, 700)
	for i := range image {
		image[i] = byte(i)
	}
	rec := newRecorder(-1, true)
	u := firmware.NewUpdater(rec, firmware.Config{ChunkSize: 100, Priority: rcp.PriorityNormal})

	if err := u.Update(context.Background(), image); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// 700 / 100 = 7 chunks
	rec.mu.Lock()
	got := len(rec.chunks)
	rec.mu.Unlock()
	if got != 7 {
		t.Errorf("chunk count = %d, want 7", got)
	}
}

// TestFirmware_CRCInPayload verifies CRC is embedded in every chunk (REQ-FW-003).
func TestFirmware_CRCInPayload(t *testing.T) {
	image := []byte("hello firmware world")
	expected := crc32.ChecksumIEEE(image)
	rec := newRecorder(-1, true)
	u := firmware.NewUpdater(rec, firmware.DefaultConfig())

	if err := u.Update(context.Background(), image); err != nil {
		t.Fatalf("Update: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, cp := range rec.chunks {
		if cp.CRC32 != expected {
			t.Errorf("chunk CRC = %08x, want %08x", cp.CRC32, expected)
		}
	}
}

// TestFirmware_ChunkFailure propagates zone error as ErrZoneRejected (REQ-FW-004).
func TestFirmware_ChunkFailure(t *testing.T) {
	image := bytes.Repeat([]byte{1}, 512)
	rec := newRecorder(1, true) // fail on 2nd chunk
	u := firmware.NewUpdater(rec, firmware.Config{ChunkSize: 128})

	err := u.Update(context.Background(), image)
	if !errors.Is(err, firmware.ErrZoneRejected) {
		t.Errorf("err = %v, want ErrZoneRejected", err)
	}
}

// TestFirmware_VerifyFailure returns ErrCRCMismatch when zone rejects verify (REQ-FW-005).
func TestFirmware_VerifyFailure(t *testing.T) {
	image := []byte("check me")
	rec := newRecorder(-1, false) // verify returns StatusError
	u := firmware.NewUpdater(rec, firmware.DefaultConfig())

	err := u.Update(context.Background(), image)
	if !errors.Is(err, firmware.ErrCRCMismatch) {
		t.Errorf("err = %v, want ErrCRCMismatch", err)
	}
}

// TestFirmware_EmptyImage rejects empty image (REQ-FW-006).
func TestFirmware_EmptyImage(t *testing.T) {
	rec := newRecorder(-1, true)
	u := firmware.NewUpdater(rec, firmware.DefaultConfig())

	err := u.Update(context.Background(), nil)
	if !errors.Is(err, firmware.ErrImageEmpty) {
		t.Errorf("err = %v, want ErrImageEmpty", err)
	}
}

// TestFirmware_ImageTooLarge rejects images over MaxImageSize (REQ-FW-006).
func TestFirmware_ImageTooLarge(t *testing.T) {
	rec := newRecorder(-1, true)
	u := firmware.NewUpdater(rec, firmware.DefaultConfig())

	oversized := make([]byte, firmware.MaxImageSize+1)
	err := u.Update(context.Background(), oversized)
	if !errors.Is(err, firmware.ErrImageTooLarge) {
		t.Errorf("err = %v, want ErrImageTooLarge", err)
	}
}

// TestFirmware_NoConcurrentUpdate rejects second simultaneous Update (REQ-FW-007).
func TestFirmware_NoConcurrentUpdate(t *testing.T) {
	// Use a blocking controller to hold the first Update in flight.
	block := make(chan struct{})
	unblock := make(chan struct{})
	first := true

	var blockCtrl blockingController
	blockCtrl.block = block
	blockCtrl.unblock = unblock
	blockCtrl.first = &first

	u := firmware.NewUpdater(&blockCtrl, firmware.Config{ChunkSize: 64})

	image := bytes.Repeat([]byte{0xFF}, 64)

	var wg sync.WaitGroup
	wg.Add(1)
	var firstErr, secondErr error
	go func() {
		defer wg.Done()
		firstErr = u.Update(context.Background(), image)
	}()

	// Wait until the first goroutine is inside Send.
	<-block
	// Second call should fail immediately with "already in progress".
	secondErr = u.Update(context.Background(), image)
	close(unblock) // unblock first goroutine
	wg.Wait()

	if firstErr != nil {
		t.Errorf("first Update: %v", firstErr)
	}
	if secondErr == nil {
		t.Error("second concurrent Update should have failed")
	}
}

// TestFirmware_MarshalRoundTrip encodes and decodes ChunkPayload correctly (REQ-FW-008).
func TestFirmware_MarshalRoundTrip(t *testing.T) {
	orig := firmware.ChunkPayload{
		TotalSize: 1024,
		Offset:    256,
		CRC32:     0xDEADBEEF,
		Data:      []byte("chunk data here"),
	}
	b := orig.Marshal()
	got, err := firmware.UnmarshalChunkPayload(b)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.TotalSize != orig.TotalSize || got.Offset != orig.Offset || got.CRC32 != orig.CRC32 {
		t.Errorf("header mismatch: got %+v, want %+v", got, orig)
	}
	if !bytes.Equal(got.Data, orig.Data) {
		t.Errorf("data mismatch")
	}
}

// TestFirmware_DefaultConfig returns expected defaults (REQ-FW-008).
func TestFirmware_DefaultConfig(t *testing.T) {
	cfg := firmware.DefaultConfig()
	if cfg.ChunkSize != firmware.DefaultChunkSize {
		t.Errorf("ChunkSize = %d, want %d", cfg.ChunkSize, firmware.DefaultChunkSize)
	}
	if cfg.Priority != rcp.PriorityHigh {
		t.Errorf("Priority = %v, want PriorityHigh", cfg.Priority)
	}
}

// blockingController blocks on the first Send until unblocked.
type blockingController struct {
	block   chan struct{}
	unblock chan struct{}
	first   *bool
}

func (b *blockingController) Send(_ context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if *b.first {
		*b.first = false
		b.block <- struct{}{} // signal that we're inside Send
		<-b.unblock           // wait to be unblocked
	}
	return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}, nil
}

func (b *blockingController) Zone() rcp.Zone                                        { return rcp.ZoneFrontLeft }
func (b *blockingController) Subscribe(_ context.Context) (<-chan *rcp.Status, error) { ch := make(chan *rcp.Status); return ch, nil }
func (b *blockingController) Close() error                                           { return nil }
