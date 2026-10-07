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
	// NoiseK and NoiseQuad are the noise model: the variance of the
	// 12-bit signal is NoiseK * bin + NoiseQuad * bin^2, where the bin is
	// its level over 64 (the noise table's), so on the 10-bit scale it is
	// NoiseK / 256 times the level in counts above black plus
	// NoiseQuad / 4096 times its square.
	NoiseK, NoiseQuad float64
	// StrengthLo and StrengthHi are the filter strengths at and below the
	// anchors' levels, and at and above them, for the R/B and the Gr/Gb
	// channels: the smaller, the stronger the filter.
	StrengthLo, StrengthHi [2]float64
	// AnchorLo and AnchorHi are the levels the strength moves between.
	AnchorLo, AnchorHi float64
}

// noiseBins is how many bins of the noise table are fitted: all of them
// before the table saturates (bin 48 on both sensors), with some margin.
const noiseBins = 40

// kTolerance is how far the noise table may stray from its fit, in variance:
// the front camera's highest gains grow more slowly than a parabola, up to
// 12% off it at the first bins. It sets a filter's width, which is not that
// precise (a 12% variance is a 6% deviation).
const kTolerance = 0.15

// noiseFit fits the table's variance, std[i]^2, to k * i + c * i^2 over its
// first bins: shot noise is k * i, and c bends the curve where the sensor's
// noise grows faster than the signal does (the front camera's highest gains).
// The fit is weighted for a relative error, as the filter's kernel is.
func noiseFit(table []float64) (k, c float64, err error) {
	if len(table) < noiseBins+1 {
		return 0, 0, fmt.Errorf("noise table of %d entries", len(table))
	}

	/* Normal equations of min sum ((v - k i - c i^2) / v)^2 */
	var saa, sab, sbb, sav, sbv float64
	for i := 1; i <= noiseBins; i++ {
		v := table[i] * table[i]
		if v <= 0 {
			return 0, 0, fmt.Errorf("noise table is zero at bin %d", i)
		}
		a, b := float64(i)/v, float64(i*i)/v
		saa += a * a
		sab += a * b
		sbb += b * b
		sav += a
		sbv += b
	}
	det := saa*sbb - sab*sab
	if det == 0 {
		return 0, 0, fmt.Errorf("noise table cannot be fitted")
	}
	k = (sav*sbb - sbv*sab) / det
	c = (saa*sbv - sab*sav) / det

	for i := 1; i <= noiseBins; i++ {
		v := table[i] * table[i]
		fit := k*float64(i) + c*float64(i*i)
		if math.Abs(fit-v) > kTolerance*v {
			return 0, 0, fmt.Errorf("noise table is not shot noise: bin %d has variance %g, the fit %g", i, v, fit)
		}
	}
	if k <= 0 {
		return 0, 0, fmt.Errorf("noise table has no shot noise")
	}

	/* A bend that is under 2% of the variance at the end of the fit is noise */
	if math.Abs(c)*noiseBins < 0.02*k {
		c = 0
	}
	return k, c, nil
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
		k, c, err := noiseFit(table)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.Name, err)
		}
		trig := obj(r, "aec_trigger")
		d := DenoiseRegion{
			GainStart: num(trig, "gain_start"),
			GainEnd:   num(trig, "gain_end"),
			NoiseK:    k,
			NoiseQuad: c,
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
