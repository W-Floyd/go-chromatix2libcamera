package convert

import (
	"fmt"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// SharpenLevels is the number of entries of the sharpening's lookup tables,
// by brightness level (the 8-bit luma over 4) or by activity.
const SharpenLevels = 64

// SharpenRegion is the adaptive spatial filter's (asf32) tuning for one gain
// region. The filter is one high-pass layer on the luma after the tone
// curve: luma + clamp(gain * detail), where the gain is that of the sign
// of the detail at the luma's level, capped, and weighted by the local
// activity, which cores noise out.
type SharpenRegion struct {
	GainStart, GainEnd float64
	// Amplitude is how strongly the kernel's band is boosted per unit of
	// gain: the kernel is fitted by a difference of Gaussians of this
	// amplitude (2.85 for the rear camera's main kernel).
	Amplitude float64
	// Positive and Negative are the gains of overshoot and undershoot
	// by brightness level; Cap limits them.
	Positive, Negative [SharpenLevels]float64
	Cap                float64
	// ClampLow and ClampHigh limit the change, in 8-bit luma units.
	ClampLow, ClampHigh float64
	// Weight is the gain's weight by activity.
	Weight [SharpenLevels]float64
}

// sharpenAmplitude is the difference of Gaussians' amplitude fitted to each
// of the three kernels the tunings use, by the kernel's centre coefficient.
var sharpenAmplitude = map[int]float64{
	1232: 2.85,
	736:  3.08,
	1968: 3.05,
}

// Sharpen reads the adaptive spatial filter's regions by gain, for an output
// at the sensor's scale and the DRC gain, from the first (default) group.
func Sharpen(f *chromatix.File, o Options) ([]SharpenRegion, error) {
	m, err := baseModule(f, "asf32")
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
	if num(root, "enable") == 0 {
		return nil, fmt.Errorf("%s is disabled", m.Name)
	}
	if str(root, "aec_exp_control") != "gain" {
		return nil, fmt.Errorf("%s is not selected by gain", m.Name)
	}

	/* The output at the sensor's scale, then the DRC gain, then non-HDR */
	scale, err := pick(list(root, "core"), "total_scale_ratio", 1, "start", "end")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Name, err)
	}
	drc, err := pick(list(scale, "total_scale_ratio_data"), "drc_gain", o.DRCGain, "start", "end")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Name, err)
	}
	hdr := list(drc, "drc_gain_data")[0].(*chromatix.Object)

	table := func(r *chromatix.Object, name string) ([SharpenLevels]float64, error) {
		var out [SharpenLevels]float64
		l := list(r, name)
		if len(l) != SharpenLevels {
			return out, fmt.Errorf("%s has %d entries", name, len(l))
		}
		for i, x := range l {
			out[i] = x.(float64)
		}
		return out, nil
	}

	var out []SharpenRegion
	for _, e := range list(hdr, "hdr_aec_data") {
		r := e.(*chromatix.Object)
		trig := obj(r, "aec_trigger")
		d := obj(r, "aec_data")

		region := SharpenRegion{
			GainStart: num(trig, "gain_start"),
			GainEnd:   num(trig, "gain_end"),
			Amplitude: 2.85,
			Cap:       num(d, "gain_cap"),
			ClampLow:  num(d, "clamp_ll"),
			ClampHigh: num(d, "clamp_ul"),
		}
		if hpf := list(d, "hpf_symmetric_coeff"); len(hpf) > 0 {
			if a, ok := sharpenAmplitude[int(hpf[len(hpf)-1].(float64))]; ok {
				region.Amplitude = a
			}
		}
		if region.Positive, err = table(d, "gain_positive_lut"); err != nil {
			return nil, fmt.Errorf("%s: %w", m.Name, err)
		}
		if region.Negative, err = table(d, "gain_negative_lut"); err != nil {
			return nil, fmt.Errorf("%s: %w", m.Name, err)
		}
		if region.Weight, err = table(d, "gain_weight_lut"); err != nil {
			return nil, fmt.Errorf("%s: %w", m.Name, err)
		}
		out = append(out, region)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no gain regions", m.Name)
	}
	return out, nil
}
