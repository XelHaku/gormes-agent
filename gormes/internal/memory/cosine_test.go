package memory

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestL2Normalize_UnitMagnitude(t *testing.T) {
	v := []float32{3, 4} // magnitude 5
	l2Normalize(v)
	mag := math.Sqrt(float64(v[0]*v[0] + v[1]*v[1]))
	if math.Abs(mag-1.0) > 1e-6 {
		t.Errorf("magnitude = %v, want 1.0 ± 1e-6", mag)
	}
}

func TestL2Normalize_ZeroVector(t *testing.T) {
	v := []float32{0, 0, 0}
	l2Normalize(v) // must not divide by zero
	for i, x := range v {
		if x != 0 {
			t.Errorf("v[%d] = %v, want 0 (zero-vector stays zero)", i, x)
		}
	}
}

func TestDotProduct_IdenticalNormalizedIsOne(t *testing.T) {
	a := []float32{0.6, 0.8} // already unit
	got := dotProduct(a, a)
	if math.Abs(float64(got)-1.0) > 1e-6 {
		t.Errorf("a·a = %v, want 1.0", got)
	}
}

func TestDotProduct_OrthogonalIsZero(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{0, 1}
	got := dotProduct(a, b)
	if got != 0 {
		t.Errorf("orthogonal = %v, want 0", got)
	}
}

func TestDotProduct_OppositeIsMinusOne(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{-1, 0}
	got := dotProduct(a, b)
	if got != -1.0 {
		t.Errorf("opposite = %v, want -1.0", got)
	}
}

func TestDotProduct_DifferentLengthsReturnsZero(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{1, 2}
	got := dotProduct(a, b)
	if got != 0 {
		t.Errorf("mismatched dim = %v, want 0 (defensive)", got)
	}
}

func TestTopK_ReturnsKHighest(t *testing.T) {
	scored := []scoredID{
		{ID: 1, Score: 0.1},
		{ID: 2, Score: 0.9},
		{ID: 3, Score: 0.5},
		{ID: 4, Score: 0.8},
	}
	got := topK(scored, 2)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// Sorted by score DESC.
	if got[0].ID != 2 || got[1].ID != 4 {
		t.Errorf("got = %+v, want [{2, 0.9}, {4, 0.8}]", got)
	}
}

func TestTopK_KLargerThanInput(t *testing.T) {
	scored := []scoredID{{ID: 1, Score: 0.5}}
	got := topK(scored, 10)
	if len(got) != 1 {
		t.Errorf("len = %d, want 1 (K > input)", len(got))
	}
}

func TestTopK_KZeroReturnsEmpty(t *testing.T) {
	scored := []scoredID{{ID: 1, Score: 0.5}}
	got := topK(scored, 0)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0 (K=0)", len(got))
	}
}

func TestEncodeFloat32LE_RoundTrip(t *testing.T) {
	in := []float32{0.1, -0.2, 3.14159, -42.5, 0}
	encoded := encodeFloat32LE(in)
	if len(encoded) != len(in)*4 {
		t.Errorf("encoded len = %d, want %d", len(encoded), len(in)*4)
	}
	out, err := decodeFloat32LE(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("decoded len = %d, want %d", len(out), len(in))
	}
	for i, want := range in {
		if out[i] != want {
			t.Errorf("out[%d] = %v, want %v", i, out[i], want)
		}
	}
}

func TestDecodeFloat32LE_OddByteLengthErrors(t *testing.T) {
	_, err := decodeFloat32LE([]byte{1, 2, 3}) // not multiple of 4
	if err == nil {
		t.Error("expected error for odd-length BLOB")
	}
}

func TestEncodeFloat32LE_LittleEndianOrder(t *testing.T) {
	// 1.0 in float32 little-endian is 0x3f800000 = bytes [0x00,0x00,0x80,0x3f]
	encoded := encodeFloat32LE([]float32{1.0})
	want := []byte{0x00, 0x00, 0x80, 0x3f}
	if !bytes.Equal(encoded, want) {
		t.Errorf("encoded = %v, want %v", encoded, want)
	}
	// Double-check against encoding/binary for future maintenance.
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, float32(1.0))
	if !bytes.Equal(encoded, buf.Bytes()) {
		t.Errorf("encoded != binary.LittleEndian write")
	}
}

func TestTopK_EmptyInput(t *testing.T) {
	got := topK([]scoredID{}, 3)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0 (empty input)", len(got))
	}
}

func TestTopK_KNegativeReturnsEmpty(t *testing.T) {
	scored := []scoredID{{ID: 1, Score: 0.5}}
	got := topK(scored, -1)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0 (K<0)", len(got))
	}
}
