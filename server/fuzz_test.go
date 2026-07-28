//fusa:test REQ-RCS-020

package server_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/regmap"
)

// ── REQ-RCS-020 (fuzz half): decoders never panic on arbitrary input ──────

func FuzzDecodeGeneralBlock(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenMinimalMap[:21]) // the fixed-length GeneralBlock prefix, see registermap.go's generalBlockLen
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _ = regmap.DecodeGeneralBlock(b) // must not panic
	})
}

func FuzzDecodeRegisterMap(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenMinimalMap)

	// A general block whose table pointers point past the end of the
	// buffer — the classic out-of-bounds trigger the pointer-ordering
	// check in DecodeRegisterMap must guard against.
	tampered := append([]byte(nil), goldenMinimalMap...)
	tampered[19], tampered[20] = 0xFF, 0xFF // EndpointTablePointer, made huge
	f.Add(tampered)

	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = regmap.DecodeRegisterMap(b) // must not panic
	})
}
