package convert

import (
	"math"
	"testing"
)

// shotTable is a noise table std = sqrt(k * i), flat from bin 48.
func shotTable(k float64) []float64 {
	t := make([]float64, 65)
	for i := range t {
		b := i
		if b > 48 {
			b = 48
		}
		t[i] = math.Sqrt(k * float64(b))
	}
	return t
}

func TestNoiseK(t *testing.T) {
	k, err := noiseK(shotTable(177))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(k-177) > 1e-6 {
		t.Errorf("k = %g, want 177", k)
	}
}

func TestNoiseKRefusesOtherCurves(t *testing.T) {
	table := shotTable(30)
	for i := range table {
		table[i] += 3 /* an offset: read noise the model does not have */
	}
	if _, err := noiseK(table); err == nil {
		t.Error("a table with an offset was taken for a shot noise model")
	}
	if _, err := noiseK(shotTable(30)[:8]); err == nil {
		t.Error("a short table was accepted")
	}
}
