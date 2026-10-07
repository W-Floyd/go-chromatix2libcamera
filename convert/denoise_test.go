package convert

import (
	"math"
	"testing"
)

// noiseTable is a noise table of variance k * bin + c * bin^2, flat from bin 48.
func noiseTable(k, c float64) []float64 {
	t := make([]float64, 65)
	for i := range t {
		b := float64(i)
		if i > 48 {
			b = 48
		}
		t[i] = math.Sqrt(k*b + c*b*b)
	}
	return t
}

func TestNoiseFitShotNoise(t *testing.T) {
	k, c, err := noiseFit(noiseTable(177, 0))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(k-177) > 1e-3 || math.Abs(c) > 1e-3 {
		t.Errorf("k, c = %g, %g, want 177, 0", k, c)
	}
}

func TestNoiseFitBentTable(t *testing.T) {
	/* The front camera's highest gains: noise grows faster than the signal */
	k, c, err := noiseFit(noiseTable(754, 3.2))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(k-754) > 0.1 || math.Abs(c-3.2) > 0.01 {
		t.Errorf("k, c = %g, %g, want 754, 3.2", k, c)
	}
}

func TestNoiseFitRefusesOtherCurves(t *testing.T) {
	table := noiseTable(30, 0)
	for i := range table {
		table[i] += 3 /* an offset: read noise the model does not have */
	}
	if _, _, err := noiseFit(table); err == nil {
		t.Error("a table with an offset was taken for a shot noise model")
	}
	if _, _, err := noiseFit(noiseTable(30, 0)[:8]); err == nil {
		t.Error("a short table was accepted")
	}
}
