package firmware

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/v9/acf"
)

// TransportFunc delivers one chunk's raw bytes over whatever underlying
// channel a caller has wired up and returns the endpoint's response — the
// same signature *udp.Controller.Write already has, so a caller can pass
// it directly via a bound closure:
//
//	transport := func(ctx context.Context, data []byte) (acf.Message, error) {
//	    return ctrl.Write(ctx, uartAddr, data)
//	}
type TransportFunc func(ctx context.Context, data []byte) (acf.Message, error)

// Sentinel errors for the firmware package.
var (
	ErrImageEmpty       = errors.New("rcp/firmware: image is empty")
	ErrImageTooLarge    = errors.New("rcp/firmware: image exceeds MaxImageSize")
	ErrCRCMismatch      = errors.New("rcp/firmware: CRC mismatch after delivery")
	ErrEndpointRejected = errors.New("rcp/firmware: endpoint rejected chunk")
	ErrUpdateInProgress = errors.New("rcp/firmware: update already in progress")
)

// MaxImageSize is the maximum accepted firmware image size (4 MiB).
const MaxImageSize = 4 << 20

// DefaultChunkSize is the default chunk payload size in bytes.
const DefaultChunkSize = 256

// Config controls firmware delivery behaviour.
type Config struct {
	// ChunkSize is the number of image bytes per chunk (default: DefaultChunkSize).
	ChunkSize int
}

// DefaultConfig returns a Config with safe defaults.
func DefaultConfig() Config {
	return Config{ChunkSize: DefaultChunkSize}
}

// ChunkPayload is the binary layout of one chunk's raw-byte-endpoint body.
// This layout is this package's own design (see doc.go); it is not defined
// by the OPEN Alliance TC18 Remote Control Protocol Specification.
//
//	[4 bytes] total image size (big-endian uint32)
//	[4 bytes] chunk offset    (big-endian uint32)
//	[4 bytes] total CRC-32    (big-endian uint32, sent on every chunk)
//	[N bytes] chunk data
type ChunkPayload struct {
	TotalSize uint32
	Offset    uint32
	CRC32     uint32
	Data      []byte
}

// Marshal encodes a ChunkPayload into a byte slice.
func (cp ChunkPayload) Marshal() []byte {
	buf := make([]byte, 12+len(cp.Data))
	binary.BigEndian.PutUint32(buf[0:4], cp.TotalSize)
	binary.BigEndian.PutUint32(buf[4:8], cp.Offset)
	binary.BigEndian.PutUint32(buf[8:12], cp.CRC32)
	copy(buf[12:], cp.Data)
	return buf
}

// UnmarshalChunkPayload decodes a payload produced by Marshal.
func UnmarshalChunkPayload(b []byte) (ChunkPayload, error) {
	if len(b) < 12 {
		return ChunkPayload{}, fmt.Errorf("rcp/firmware: payload too short (%d bytes)", len(b))
	}
	cp := ChunkPayload{
		TotalSize: binary.BigEndian.Uint32(b[0:4]),
		Offset:    binary.BigEndian.Uint32(b[4:8]),
		CRC32:     binary.BigEndian.Uint32(b[8:12]),
		Data:      make([]byte, len(b)-12),
	}
	copy(cp.Data, b[12:])
	return cp, nil
}

// Updater delivers a firmware image over one TransportFunc.
type Updater struct {
	transport TransportFunc
	cfg       Config
	active    atomic.Bool
}

// NewUpdater returns an Updater delivering chunks via transport.
func NewUpdater(transport TransportFunc, cfg Config) *Updater {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = DefaultChunkSize
	}
	return &Updater{transport: transport, cfg: cfg}
}

// Update delivers image via the Updater's TransportFunc and verifies the
// CRC. Only one Update may be in progress per Updater at a time;
// concurrent calls return ErrUpdateInProgress immediately.
func (u *Updater) Update(ctx context.Context, image []byte) error {
	if len(image) == 0 {
		return ErrImageEmpty
	}
	if len(image) > MaxImageSize {
		return ErrImageTooLarge
	}
	if !u.active.CompareAndSwap(false, true) {
		return ErrUpdateInProgress
	}
	defer u.active.Store(false)

	crc := crc32.ChecksumIEEE(image)
	total := uint32(len(image))
	chunkSize := u.cfg.ChunkSize

	for offset := 0; offset < len(image); offset += chunkSize {
		end := offset + chunkSize
		if end > len(image) {
			end = len(image)
		}
		chunk := image[offset:end]
		cp := ChunkPayload{
			TotalSize: total,
			Offset:    uint32(offset),
			CRC32:     crc,
			Data:      chunk,
		}
		resp, err := u.transport(ctx, cp.Marshal())
		if err != nil {
			return fmt.Errorf("rcp/firmware: chunk @%d: %w", offset, err)
		}
		if resp.Control.Has(acf.FlagError) {
			return fmt.Errorf("rcp/firmware: chunk @%d: %w", offset, ErrEndpointRejected)
		}
	}

	// Post-install verification: send a zero-length chunk with offset ==
	// total as a sentinel meaning "verify"; the endpoint echoes back a
	// non-FlagError response only if its received image matches.
	verify := ChunkPayload{TotalSize: total, Offset: total, CRC32: crc}
	resp, err := u.transport(ctx, verify.Marshal())
	if err != nil {
		return fmt.Errorf("rcp/firmware: verify: %w", err)
	}
	if resp.Control.Has(acf.FlagError) {
		return ErrCRCMismatch
	}
	return nil
}
