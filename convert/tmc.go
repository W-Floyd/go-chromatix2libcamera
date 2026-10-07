package convert

import (
	"fmt"
	"sort"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// DRCAnchors are TMC 1.2's tone anchors at a DRC gain: the dark, mid and
// bright anchors (tone_anchors[1..3]) and the dark boost offset.
type DRCAnchors struct {
	DRCGain           float64
	Dark, Mid, Bright float64
	// DarkBoost is dark_boost_offset; DarkBoostRatio, dark_boost_ratio,
	// weighs AEC's dark boost gain into the shadows' gain.
	DarkBoost, DarkBoostRatio float64
}

// DRCCurve is TMC 1.2's DRC tone mapping from its anchor knee points.
type DRCCurve struct {
	// MaxRatio is the largest gain, tone_max_ratio.
	MaxRatio float64
	// Anchors are by increasing DRC gain.
	Anchors []DRCAnchors
	// GTMShare is the share of the curve CamX applies globally, the rest
	// by local tone mapping (gtm_percentage, of the lowest DRC region).
	GTMShare float64
	// ByLux are the curves of every lux index region.
	ByLux []LuxDRC
}

// DRCToneCurve reads tmc12_sw's anchors for the selected lux region, at the
// start and end of each DRC gain region, so that they are flat within a
// region and interpolated between regions, as CamX does. Only the anchor
// knee points are converted: CamX blends in histogram knee points by
// hist_enhance_ratio, and a histogram contrast curve by contrast_he_*,
// and those are refused when used beyond a small share.
func DRCToneCurve(f *chromatix.File, o Options) (*DRCCurve, error) {
	var mod *chromatix.Symbol
	for _, m := range f.Modules() {
		if m.Name == "tmc12_sw" {
			mm := m
			mod = &mm
			break
		}
	}
	if mod == nil {
		return nil, fmt.Errorf("no tmc12_sw module")
	}
	t, ok := isp.Schema(*mod)
	if !ok {
		return nil, fmt.Errorf("no schema for %s %s", mod.Name, mod.VersionString())
	}
	v, err := f.Decode(*mod, t)
	if err != nil {
		return nil, err
	}
	root := v.(*chromatix.Object)
	if num(root, "enable") == 0 || num(root, "use_gtm") == 0 {
		return nil, fmt.Errorf("%s: global tone mapping is disabled", mod.Name)
	}

	out, err := drcAt(root, mod.Name, o.LuxIdx)
	if err != nil {
		return nil, err
	}

	/*
	 * The curves of the other lux regions, as the first DRC gain region
	 * has them (the others' boundaries may differ a little).
	 */
	if str(root, "aec_exp_control") == "lux_idx" {
		first := list(root, "core")[0].(*chromatix.Object)
		hdr := list(first, "drc_gain_data")[0].(*chromatix.Object)
		for _, e := range list(hdr, "hdr_aec_data") {
			t := obj(e.(*chromatix.Object), "aec_trigger")
			start, end := num(t, "lux_idx_start"), num(t, "lux_idx_end")
			c, err := drcAt(root, mod.Name, start)
			if err != nil {
				return nil, err
			}
			out.ByLux = append(out.ByLux, LuxDRC{Start: start, End: end, Curve: c})
		}
		sort.SliceStable(out.ByLux, func(i, j int) bool { return out.ByLux[i].Start < out.ByLux[j].Start })
	}
	return out, nil
}

// LuxDRC is the DRC tone mapping of one lux index region.
type LuxDRC struct {
	Start, End float64
	Curve      *DRCCurve
}

// drcAt reads the anchors of the lux region containing a lux index.
func drcAt(root *chromatix.Object, module string, lux float64) (*DRCCurve, error) {
	var err error

	out := &DRCCurve{MaxRatio: num(root, "tone_max_ratio"), GTMShare: -1}
	byLux := str(root, "aec_exp_control") == "lux_idx"
	for _, e := range list(root, "core") {
		d := e.(*chromatix.Object)
		trig := obj(d, "drc_gain_trigger")
		/* HDR ratio: the first region is the non-HDR one */
		hdr := list(d, "drc_gain_data")[0].(*chromatix.Object)
		var aec *chromatix.Object
		if byLux {
			aec, err = pick(list(hdr, "hdr_aec_data"), "aec", lux, "lux_idx_start", "lux_idx_end")
		} else {
			aec, err = pick(list(hdr, "hdr_aec_data"), "aec", 1, "gain_start", "gain_end")
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", module, err)
		}
		r := obj(aec, "aec_data")
		for _, name := range []string{"hist_enhance_ratio"} {
			for _, x := range list(r, name) {
				if x.(float64) != 0 {
					return nil, fmt.Errorf("%s: %s is used; only anchor knee points convert", module, name)
				}
			}
		}
		tones := list(r, "tone_anchors")
		a := DRCAnchors{
			Dark:           tones[1].(float64),
			Mid:            tones[2].(float64),
			Bright:         tones[3].(float64),
			DarkBoost:      num(r, "dark_boost_offset"),
			DarkBoostRatio: num(r, "dark_boost_ratio"),
		}
		for _, g := range []float64{num(trig, "start"), num(trig, "end")} {
			a.DRCGain = g
			out.Anchors = append(out.Anchors, a)
		}
		if out.GTMShare < 0 {
			out.GTMShare = num(r, "gtm_percentage")
		}
	}
	sort.SliceStable(out.Anchors, func(i, j int) bool { return out.Anchors[i].DRCGain < out.Anchors[j].DRCGain })
	if len(out.Anchors) == 0 {
		return nil, fmt.Errorf("%s: no DRC gain regions", module)
	}
	return out, nil
}
