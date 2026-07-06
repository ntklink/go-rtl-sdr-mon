package sdr

import (
	"math"
	"testing"
)

// The streaming Resampler maintains one sample of history (lastSample) so
// linear interpolation stays continuous across block boundaries. This
// introduces a constant one-input-sample group delay, which is expected and
// correct for a streaming resampler.

func TestResamplerPassthroughDelay(t *testing.T) {
	// Same rate (no anti-alias filter). Output is the input delayed by one
	// sample, with a leading zero from the initial lastSample.
	r := NewResampler(48000, 48000)
	in1 := []float64{10, 20, 30, 40}
	in2 := []float64{50, 60, 70, 80}
	out1 := r.Process(in1)
	out2 := r.Process(in2)
	joined := append(out1, out2...)

	// joined[0] == 0 (startup), then joined[i+1] == stream[i] where
	// stream = in1[0..3] ++ in2[0..3].
	stream := append(append([]float64{}, in1...), in2...)
	if math.Abs(joined[0]) > 1e-9 {
		t.Errorf("startup out[0]: got %v, want 0", joined[0])
	}
	for i := 0; i < len(stream)-1; i++ {
		if math.Abs(joined[i+1]-stream[i]) > 1e-9 {
			t.Errorf("delayed out[%d]: got %v, want %v", i+1, joined[i+1], stream[i])
		}
	}
}

func TestResamplerUpsampleIntegerPositions(t *testing.T) {
	// 1:2 upsample (no anti-alias filter). Integer input positions must map
	// to the correct input sample (no off-by-one), with midpoints between.
	r := NewResampler(100, 200) // ratio 2 -> step 0.5, no aaFilter (upsample)
	in := []float64{10, 20, 30, 40}
	out := r.Process(in)

	// Positions 0,0.5,1,1.5,2,2.5,3,3.5 over extended [0, in[0..3]].
	// in[0] appears at output index 2, in[1] at 4, in[2] at 6.
	want := []float64{0, 5, 10, 15, 20, 25, 30, 35}
	if len(out) != len(want) {
		t.Fatalf("upsample length: got %d, want %d", len(out), len(want))
	}
	for i, w := range want {
		if math.Abs(out[i]-w) > 1e-9 {
			t.Errorf("upsample out[%d]: got %v, want %v", i, out[i], w)
		}
	}
}

func TestResampleComplexPassthrough(t *testing.T) {
	in := make([]complex128, 8)
	for i := range in {
		in[i] = complex(float64(i), float64(i)*2)
	}
	out := ResampleComplex(in, 100, 100) // same rate -> returned as-is
	if &in[0] != &out[0] {
		t.Error("same-rate ResampleComplex should return input unchanged")
	}
}

func TestResampleComplexUpsample(t *testing.T) {
	// 1:2 upsample. At integer input positions the output must equal the input
	// (verifying the off-by-one fix).
	in := []complex128{1 + 0i, 2 + 0i, 3 + 0i, 4 + 0i}
	out := ResampleComplex(in, 100, 200) // ratio 2 -> step 0.5
	// Outputs at phase 0,0.5,1,1.5,2,... -> in[0], mid, in[1], mid, in[2], ...
	if math.Abs(real(out[0])-1) > 1e-9 {
		t.Errorf("out[0]: got %v, want 1", real(out[0]))
	}
	if math.Abs(real(out[2])-2) > 1e-9 {
		t.Errorf("out[2]: got %v, want 2 (in[1])", real(out[2]))
	}
	if math.Abs(real(out[4])-3) > 1e-9 {
		t.Errorf("out[4]: got %v, want 3 (in[2])", real(out[4]))
	}
	if math.Abs(real(out[1])-1.5) > 1e-9 {
		t.Errorf("out[1]: got %v, want 1.5 (midpoint)", real(out[1]))
	}
}
