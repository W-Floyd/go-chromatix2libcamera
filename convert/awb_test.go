package convert

import (
	"math"
	"testing"
)

func TestTransverse(t *testing.T) {
	// A straight locus from (1, 0.5) to (0.5, 1); its normal away from
	// the origin is (1, 1)/sqrt 2.
	locus := []WhitePoint{{CT: 3000, RG: 1, BG: 0.5}, {CT: 6000, RG: 0.5, BG: 1}}
	other := []WhitePoint{
		{Name: "purple", RG: 0.8, BG: 0.8},    // 0.1/sqrt 2 out
		{Name: "green", RG: 0.7, BG: 0.7},     // 0.05/sqrt 2 in
		{Name: "near", RG: 0.751, BG: 0.751},  // within the minimum
		{Name: "greener", RG: 0.65, BG: 0.65}, // 0.2/sqrt 2 in
	}
	pos, neg := Transverse(locus, other)
	if math.Abs(pos-0.1/math.Sqrt2) > 1e-9 || math.Abs(neg-0.2/math.Sqrt2) > 1e-9 {
		t.Errorf("pos %g neg %g", pos, neg)
	}

	pos, neg = Transverse(locus, nil)
	if pos != MinTransverse || neg != MinTransverse {
		t.Errorf("no off-locus points: pos %g neg %g", pos, neg)
	}
}
