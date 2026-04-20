package memory

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// scoredID is a (entity_id, similarity) pair — the output of the
// similarity scan, consumable by topK.
type scoredID struct {
	ID    int64
	Score float32
}

// l2Normalize rescales v in-place to unit magnitude. A zero vector
// stays zero (defensive — avoids NaN from 0/0). Callers must ensure
// no element is NaN or ±Inf before calling; those values propagate
// silently (no guard in this function).
func l2Normalize(v []float32) {
	var sumSq float32
	for _, x := range v {
		sumSq += x * x
	}
	if sumSq == 0 {
		return
	}
	inv := float32(1.0 / math.Sqrt(float64(sumSq)))
	for i := range v {
		v[i] *= inv
	}
}

// dotProduct returns sum(a[i]*b[i]). For L2-normalized vectors this
// IS the cosine similarity. Defensively returns 0 on mismatched dim
// rather than panicking — corrupt rows shouldn't crash recall.
func dotProduct(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot float32
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot
}

// topK returns the K highest-scoring entries, sorted score-descending.
// For K >= len(scored), returns all entries. For K <= 0, returns empty.
// Runs in O(n log n) via a simple sort — for Gormes scale (≤10k entities,
// K=3), the dedicated min-heap optimization would save microseconds at
// the cost of code complexity.
func topK(scored []scoredID, k int) []scoredID {
	if k <= 0 {
		return nil
	}
	out := make([]scoredID, len(scored))
	copy(out, scored)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	if len(out) > k {
		out = out[:k]
	}
	return out
}

// encodeFloat32LE packs a float32 slice into a BLOB of little-endian
// bytes (4 bytes per float). Used for entity_embeddings.vec storage.
func encodeFloat32LE(v []float32) []byte {
	out := make([]byte, 4*len(v))
	for i, f := range v {
		bits := math.Float32bits(f)
		binary.LittleEndian.PutUint32(out[i*4:], bits)
	}
	return out
}

// decodeFloat32LE is the inverse of encodeFloat32LE. Returns an error
// if the input length isn't a multiple of 4.
func decodeFloat32LE(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("memory: decodeFloat32LE: length %d not multiple of 4", len(b))
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		out[i] = math.Float32frombits(bits)
	}
	return out, nil
}
