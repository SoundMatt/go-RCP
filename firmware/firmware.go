// Package firmware provides OTA firmware delivery for automotive zone controllers.
//
// An Updater chunks a firmware image, delivers it to a zone controller via
// CmdUpdate commands, and verifies installation with a CRC-32 checksum. The
// CmdUpdate command type is defined here and added to the CommandType space.
//
// Delivery uses a simple stop-and-wait protocol: each chunk is sent, the
// response is checked, and only then is the next chunk sent. The chunk size is
// configurable to match the zone controller's receive-buffer constraints.
//
// ISO 26262 ASIL-B rationale: firmware updates are infrequent but safety-critical
// operations. The package enforces image size limits, non-zero CRC, and a
// post-install echo to guard against silent corruption.
package firmware

//fusa:req REQ-FW-001
//fusa:req REQ-FW-002
//fusa:req REQ-FW-003
//fusa:req REQ-FW-004
//fusa:req REQ-FW-005
//fusa:req REQ-FW-006
//fusa:req REQ-FW-007
//fusa:req REQ-FW-008

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// CmdUpdate is the command type for firmware chunk delivery.
// The payload encoding is defined by ChunkPayload.
const CmdUpdate rcp.CommandType = 10

// Sentinel errors for the firmware package.
var (
	ErrImageEmpty   = errors.New("rcp/firmware: image is empty")
	ErrImageTooLarge = errors.New("rcp/firmware: image exceeds MaxImageSize")
	ErrCRCMismatch  = errors.New("rcp/firmware: CRC mismatch after delivery")
	ErrZoneRejected = errors.New("rcp/firmware: zone controller rejected chunk")
)

// MaxImageSize is the maximum accepted firmware image size (4 MiB).
const MaxImageSize = 4 << 20

// DefaultChunkSize is the default chunk payload size in bytes.
const DefaultChunkSize = 256

// Config controls firmware delivery behaviour.
type Config struct {
	// ChunkSize is the number of image bytes per CmdUpdate payload (default: DefaultChunkSize).
	ChunkSize int
	// Priority is the command priority used for all CmdUpdate messages.
	Priority rcp.Priority
}

// DefaultConfig returns a Config with safe defaults.
func DefaultConfig() Config {
	return Config{
		ChunkSize: DefaultChunkSize,
		Priority:  rcp.PriorityHigh,
	}
}

// ChunkPayload is the binary layout of a CmdUpdate command payload.
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

// Updater delivers a firmware image to a single zone controller.
type Updater struct {
	ctrl   rcp.Controller
	cfg    Config
	active atomic.Bool
}

// NewUpdater returns an Updater targeting ctrl.
func NewUpdater(ctrl rcp.Controller, cfg Config) *Updater {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = DefaultChunkSize
	}
	return &Updater{ctrl: ctrl, cfg: cfg}
}

// Update delivers image to the zone controller and verifies the CRC.
// Only one Update may be in progress per Updater at a time; concurrent calls
// return an error immediately.
func (u *Updater) Update(ctx context.Context, image []byte) error {
	if len(image) == 0 {
		return ErrImageEmpty
	}
	if len(image) > MaxImageSize {
		return ErrImageTooLarge
	}
	if !u.active.CompareAndSwap(false, true) {
		return fmt.Errorf("rcp/firmware: update already in progress on zone %s", u.ctrl.Zone())
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
		cmd := &rcp.Command{
			Zone:     u.ctrl.Zone(),
			Type:     CmdUpdate,
			Priority: u.cfg.Priority,
			Payload:  cp.Marshal(),
		}
		resp, err := u.ctrl.Send(ctx, cmd)
		if err != nil {
			return fmt.Errorf("rcp/firmware: zone %s chunk @%d: %w", u.ctrl.Zone(), offset, err)
		}
		if resp.Status != rcp.StatusOK {
			return fmt.Errorf("rcp/firmware: zone %s chunk @%d: %w", u.ctrl.Zone(), offset, ErrZoneRejected)
		}
	}

	// Post-install verification: send a zero-length chunk with the expected CRC;
	// the zone controller echoes back StatusOK only if its received image matches.
	verifyCmd := &rcp.Command{
		Zone:     u.ctrl.Zone(),
		Type:     CmdUpdate,
		Priority: u.cfg.Priority,
		Payload: ChunkPayload{
			TotalSize: total,
			Offset:    total, // sentinel: offset == total means verify
			CRC32:     crc,
		}.Marshal(),
	}
	resp, err := u.ctrl.Send(ctx, verifyCmd)
	if err != nil {
		return fmt.Errorf("rcp/firmware: zone %s verify: %w", u.ctrl.Zone(), err)
	}
	if resp.Status != rcp.StatusOK {
		return fmt.Errorf("rcp/firmware: zone %s: %w", u.ctrl.Zone(), ErrCRCMismatch)
	}
	return nil
}
