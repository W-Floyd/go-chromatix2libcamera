package convert

import (
	"math"
	"testing"
)

// The S5KJNS's exposure table: index 23.45 per stop of gain x exposure time.
var s5kjnsKnees = []KneeEntry{
	{1, 31619, 0},
	{2, 5000000, 194.7502},
	{8, 10000000, 265.0995},
	{16, 20000000, 311.9991},
	{32, 30000000, 349.1661},
	{64, 50000000, 389.8975},
	{94, 60000000, 409.0706},
	{128, 72000000, 425.6835},
	{196, 72000000, 440.0983},
	{216, 72000000, 443.3854},
}

func TestLuxFromKnees(t *testing.T) {
	m, err := LuxFromKnees(s5kjnsKnees)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(m.PerStop-1/math.Log2(1.03)) > 0.01 {
		t.Errorf("PerStop = %g, want a 3%% step, %g", m.PerStop, 1/math.Log2(1.03))
	}
	/* 250 is at 31.619 us x 1.03^250 */
	if want := 31.619 * math.Pow(1.03, 250); math.Abs(m.RefProduct-want)/want > 0.01 {
		t.Errorf("RefProduct = %g, want %g", m.RefProduct, want)
	}
}

func TestLuxFromKneesRefusesAMisfit(t *testing.T) {
	k := append([]KneeEntry(nil), s5kjnsKnees...)
	k[4].Index = 100
	if _, err := LuxFromKnees(k); err == nil {
		t.Error("a knee far off the fit was accepted")
	}
	if _, err := LuxFromKnees(k[:1]); err == nil {
		t.Error("one knee was accepted")
	}
}
