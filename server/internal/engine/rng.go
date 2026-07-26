package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"math/rand/v2"
)

// Stream derives an independent, reproducible random stream from the room
// seed and a label path. All engine randomness must come from here.
func Stream(seed uint64, labels ...string) *rand.Rand {
	h := sha256.New()
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], seed)
	h.Write(buf[:])
	for _, l := range labels {
		h.Write([]byte{0})
		h.Write([]byte(l))
	}
	sum := h.Sum(nil)
	return rand.New(rand.NewPCG(
		binary.LittleEndian.Uint64(sum[0:8]),
		binary.LittleEndian.Uint64(sum[8:16]),
	))
}
