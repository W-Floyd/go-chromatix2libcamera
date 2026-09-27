package convert

import (
	"math"
	"testing"
)

func TestTransverse(t *testing.T) {
	// A straight locus (1, 0.5) 3000 K, (0.75, 0.75) 4500 K, (0.5, 1)
	// 6000 K; its normal away from the origin is (1, 1)/sqrt 2.
	locus := []WhitePoint{
		{CT: 3000, RG: 1, BG: 0.5},
		{CT: 4500, RG: 0.75, BG: 0.75},
		{CT: 6000, RG: 0.5, BG: 1},
	}
	other := []WhitePoint{
		{Name: "purple", RG: 0.9, BG: 0.7},   // 0.1/sqrt 2 out, first segment
		{Name: "green", RG: 0.7, BG: 0.7},    // 0.05/sqrt 2 in, at 4500 K
		{Name: "near", RG: 0.551, BG: 0.951}, // within the minimum
		{Name: "greener", RG: 0.5, BG: 0.9},  // 0.1/sqrt 2 in, second segment
	}
	pos, neg := Transverse(locus, other)
	want := func(band []BandPoint, widths ...float64) {
		t.Helper()
		for i, w := range widths {
			if band[i].CT != locus[i].CT || math.Abs(band[i].Width-w) > 1e-9 {
				t.Errorf("point %d: %d K %g, want %d K %g", i, band[i].CT, band[i].Width, locus[i].CT, w)
			}
		}
	}
	d := 0.1 / math.Sqrt2
	want(pos, d, d, MinTransverse)
	want(neg, MinTransverse, math.Max(0.05/math.Sqrt2, d), d)

	pos, neg = Transverse(locus, nil)
	want(pos, MinTransverse, MinTransverse, MinTransverse)
	want(neg, MinTransverse, MinTransverse, MinTransverse)
}
