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
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/firmware"
)

// TestUpdate_RejectsEmptyImage verifies Update refuses a zero-length image
// (REQ-FW-001).
func TestUpdate_RejectsEmptyImage(t *testing.T) {
	u := firmware.NewUpdater(func(context.Context, []byte) (acf.Message, error) {
		return acf.Message{}, nil
	}, firmware.DefaultConfig())
	if err := u.Update(context.Background(), nil); !errors.Is(err, firmware.ErrImageEmpty) {
		t.Errorf("err = %v, want ErrImageEmpty", err)
	}
}

// TestUpdate_RejectsOversizedImage verifies Update refuses an image larger
// than MaxImageSize (REQ-FW-002).
func TestUpdate_RejectsOversizedImage(t *testing.T) {
	u := firmware.NewUpdater(func(context.Context, []byte) (acf.Message, error) {
		return acf.Message{}, nil
	}, firmware.DefaultConfig())
	big := make([]byte, firmware.MaxImageSize+1)
	if err := u.Update(context.Background(), big); !errors.Is(err, firmware.ErrImageTooLarge) {
		t.Errorf("err = %v, want ErrImageTooLarge", err)
	}
}

// TestUpdate_DeliversChunksInOrderWithConsistentCRC verifies every
// delivered chunk carries the whole image's CRC-32 and offsets advance by
// ChunkSize (REQ-FW-003).
func TestUpdate_DeliversChunksInOrderWithConsistentCRC(t *testing.T) {
	var chunks []firmware.ChunkPayload
	transport := func(_ context.Context, data []byte) (acf.Message, error) {
		cp, err := firmware.UnmarshalChunkPayload(data)
		if err != nil {
			t.Fatalf("UnmarshalChunkPayload: %v", err)
		}
		chunks = append(chunks, cp)
		return acf.Message{Control: acf.FlagResponse}, nil
	}
	u := firmware.NewUpdater(transport, firmware.Config{ChunkSize: 4})
	image := []byte("0123456789") // 3 chunks of 4,4,2 bytes + 1 verify call
	if err := u.Update(context.Background(), image); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// 3 data chunks + 1 verify call = 4.
	if len(chunks) != 4 {
		t.Fatalf("len(chunks) = %d, want 4", len(chunks))
	}
	wantCRC := chunks[0].CRC32
	for i, c := range chunks[:3] {
		if c.CRC32 != wantCRC {
			t.Errorf("chunk %d CRC32 = %d, want %d (consistent across all chunks)", i, c.CRC32, wantCRC)
		}
		if c.Offset != uint32(i*4) {
			t.Errorf("chunk %d Offset = %d, want %d", i, c.Offset, i*4)
		}
	}
	// Verify call is the sentinel: Offset == TotalSize.
	verify := chunks[3]
	if verify.Offset != verify.TotalSize {
		t.Errorf("verify call Offset = %d, want %d (== TotalSize)", verify.Offset, verify.TotalSize)
	}
}

// TestUpdate_EndpointRejectsChunk_StopsDelivery verifies a FlagError
// response mid-delivery aborts Update with ErrEndpointRejected and no
// further chunks are sent (REQ-FW-004).
func TestUpdate_EndpointRejectsChunk_StopsDelivery(t *testing.T) {
	calls := 0
	transport := func(_ context.Context, data []byte) (acf.Message, error) {
		calls++
		cp, _ := firmware.UnmarshalChunkPayload(data)
		if cp.Offset == 4 {
			return acf.Message{Control: acf.FlagResponse | acf.FlagError}, nil
		}
		return acf.Message{Control: acf.FlagResponse}, nil
	}
	u := firmware.NewUpdater(transport, firmware.Config{ChunkSize: 4})
	err := u.Update(context.Background(), []byte("0123456789"))
	if !errors.Is(err, firmware.ErrEndpointRejected) {
		t.Errorf("err = %v, want ErrEndpointRejected", err)
	}
	if calls != 2 {
		t.Errorf("transport called %d times, want 2 (stopped at the rejected chunk)", calls)
	}
}

// TestUpdate_VerifyRejected_ReportsCRCMismatch verifies a FlagError
// response to the post-install verify call reports ErrCRCMismatch
// (REQ-FW-005).
func TestUpdate_VerifyRejected_ReportsCRCMismatch(t *testing.T) {
	transport := func(_ context.Context, data []byte) (acf.Message, error) {
		cp, _ := firmware.UnmarshalChunkPayload(data)
		if cp.Offset == cp.TotalSize {
			return acf.Message{Control: acf.FlagResponse | acf.FlagError}, nil
		}
		return acf.Message{Control: acf.FlagResponse}, nil
	}
	u := firmware.NewUpdater(transport, firmware.DefaultConfig())
	err := u.Update(context.Background(), []byte("hello"))
	if !errors.Is(err, firmware.ErrCRCMismatch) {
		t.Errorf("err = %v, want ErrCRCMismatch", err)
	}
}

// TestUpdate_Success_NoError verifies a fully-acknowledged delivery and
// verify call reports no error (REQ-FW-006).
func TestUpdate_Success_NoError(t *testing.T) {
	transport := func(context.Context, []byte) (acf.Message, error) {
		return acf.Message{Control: acf.FlagResponse}, nil
	}
	u := firmware.NewUpdater(transport, firmware.DefaultConfig())
	if err := u.Update(context.Background(), []byte("firmware image bytes")); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

// TestUpdate_ConcurrentCalls_SecondRejected verifies a second concurrent
// Update call on the same Updater reports ErrUpdateInProgress (REQ-FW-007).
func TestUpdate_ConcurrentCalls_SecondRejected(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	transport := func(context.Context, []byte) (acf.Message, error) {
		once.Do(func() { close(started) })
		<-release
		return acf.Message{Control: acf.FlagResponse}, nil
	}
	u := firmware.NewUpdater(transport, firmware.Config{ChunkSize: 1})

	var wg sync.WaitGroup
	var firstErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		firstErr = u.Update(context.Background(), []byte("ab"))
	}()

	<-started
	secondErr := u.Update(context.Background(), []byte("cd"))
	if !errors.Is(secondErr, firmware.ErrUpdateInProgress) {
		t.Errorf("second Update err = %v, want ErrUpdateInProgress", secondErr)
	}
	close(release)
	wg.Wait()
	if firstErr != nil {
		t.Errorf("first Update err = %v, want nil", firstErr)
	}
}

// TestChunkPayload_MarshalUnmarshal_RoundTrips verifies Marshal/
// UnmarshalChunkPayload round-trip every field (REQ-FW-008).
func TestChunkPayload_MarshalUnmarshal_RoundTrips(t *testing.T) {
	want := firmware.ChunkPayload{TotalSize: 100, Offset: 8, CRC32: 0xDEADBEEF, Data: []byte("chunk")}
	got, err := firmware.UnmarshalChunkPayload(want.Marshal())
	if err != nil {
		t.Fatalf("UnmarshalChunkPayload: %v", err)
	}
	if got.TotalSize != want.TotalSize || got.Offset != want.Offset || got.CRC32 != want.CRC32 || string(got.Data) != string(want.Data) {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, want)
	}
}
