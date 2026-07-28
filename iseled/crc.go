package iseled

// crcPoly, crcInit, and crcXorOut are this package's own reasoned CRC-8
// parameter choice — see doc.go's spec-fidelity note for why this is not a
// verified transcription of ISELED's own native CRC algorithm.
const (
	crcPoly   = 0x1D
	crcInit   = 0xFF
	crcXorOut = 0xFF
)

// ComputeCRC computes this package's ISELED-native CRC8 over data: a
// bit-by-bit, MSB-first CRC8 using crcPoly/crcInit/crcXorOut. See doc.go's
// "ISELED-native CRC" section for how this composes with (rather than
// replaces) the general e2e end-to-end mechanism.
func ComputeCRC(data []byte) uint8 {
	crc := uint8(crcInit)
	for _, b := range data {
		crc ^= b
		for i := 0; i < 8; i++ {
			if crc&0x80 != 0 {
				crc = (crc << 1) ^ crcPoly
			} else {
				crc <<= 1
			}
		}
	}
	return crc ^ crcXorOut
}
