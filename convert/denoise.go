package convert

import (
	"fmt"
	"math"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// DenoiseRegion is the adaptive Bayer filter's tuning (bpcabf41) for one
// gain region.
type DenoiseRegion struct {
	GainStart, GainEnd float64
	// NoiseK is the slope of the noise model: the variance of the
	// 12-bit signal is NoiseK times its level over 64 (the noise table's
	// bin), so on the 10-bit scale it is NoiseK / 256 times the level in
	// counts above black.
	NoiseK float64
	// StrengthLo and StrengthHi are the filter strengths at and below the
	// anchors' levels, and at and above them, for the R/B and the Gr/Gb
	// channels: the smaller, the stronger the filter.
	StrengthLo, StrengthHi [2]float64
	// AnchorLo and AnchorHi are the levels the strength moves between.
	AnchorLo, AnchorHi float64
}

// kTolerance is how far the noise table may stray from a shot noise model
// std^2 = k * bin, over the bins before it saturates.
const kTolerance = 0.02

// noiseK fits k to the table's bins that are not saturated: std[i]^2 = k * i.
func noiseK(table []float64) (float64, error) {
	if len(table) < 17 {
		return 0, fmt.Errorf("noise table of %d entries", len(table))
	}
	k := table[1] * table[1]
	for i := 1; i <= 16; i++ {
		if got := table[i] * table[i] / float64(i); math.Abs(got-k) > kTolerance*k {
			return 0, fmt.Errorf("noise table is not a shot noise model: bin %d has %g, not %g", i, got, k)
		}
	}
	return k, nil
}

// Denoise reads the adaptive Bayer filter's regions by gain, at the DRC
// gain and from the first (default) group of the module. CamX's noise
// table is the sensor's calibration; its filter strengths are tuned.
func Denoise(f *chromatix.File, o Options) ([]DenoiseRegion, error) {
	m, err := baseModule(f, "bpcabf41")
	if err != nil {
		return nil, err
	}
	t, ok := isp.Schema(m)
	if !ok {
		return nil, fmt.Errorf("no schema for %s %s", m.Name, m.VersionString())
	}
	v, err := f.Decode(m, t)
	if err != nil {
		return nil, err
	}
	root := v.(*chromatix.Object)
	if str(root, "aec_exp_control") != "gain" {
		return nil, fmt.Errorf("%s is not selected by gain", m.Name)
	}

	drc, err := pick(list(root, "core"), "drc_gain", o.DRCGain, "start", "end")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Name, err)
	}
	/* HDR ratio: the first region is the non-HDR one */
	hdr := list(drc, "drc_gain_data")[0].(*chromatix.Object)

	floats := func(r *chromatix.Object, name string) []float64 {
		var out []float64
		for _, x := range list(r, name) {
			out = append(out, x.(float64))
		}
		return out
	}

	var out []DenoiseRegion
	for _, e := range list(hdr, "hdr_aec_data") {
		r := e.(*chromatix.Object)
		table := floats(r, "noise_std_lut_level")
		k, err := noiseK(table)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.Name, err)
		}
		trig := obj(r, "aec_trigger")
		d := DenoiseRegion{
			GainStart: num(trig, "gain_start"),
			GainEnd:   num(trig, "gain_end"),
			NoiseK:    k,
			AnchorLo:  num(r, "filter_strength_anchor_pt_lo"),
			AnchorHi:  num(r, "filter_strength_anchor_pt_hi"),
		}
		lo, hi := floats(r, "filter_strength_lo"), floats(r, "filter_strength_hi")
		if len(lo) < 2 || len(hi) < 2 {
			return nil, fmt.Errorf("%s: filter strengths of %d and %d entries", m.Name, len(lo), len(hi))
		}
		copy(d.StrengthLo[:], lo)
		copy(d.StrengthHi[:], hi)
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no gain regions", m.Name)
	}
	return out, nil
}
