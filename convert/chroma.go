package convert

import (
	"fmt"
	"math"
	"sort"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// ChromaSet is the chroma enhancement at one colour temperature, on BT.601
// chroma after the tone curve, by sign: Cb positive (blue) and negative
// (yellow), Cr positive (red) and negative (green). Scale is the chroma's
// gain (1 is none); Shear adds that much of R - G to Cb, or of B - G to
// Cr, before it (0 is none), in the soft ISP's terms:
//
//	Cb' = Scale * (Cb + Shear * (R - G))
//	Cr' = Scale * (Cr + Shear * (B - G))
type ChromaSet struct {
	CT           int
	Scale, Shear [4]float64
}

// cvTolerance is how far a luma weight may be from BT.601's and still be
// taken as it: the tuning stores them to three decimals.
const cvTolerance = 0.002

// Chroma extracts cv12's chroma enhancement for the selected region, one
// set per colour temperature. CamX applies it after gamma, as the soft ISP
// does. In CamX's terms Cb = a * ((B - G) + b * (R - G)): with a = 0.5 and
// b = CVShearB that is BT.601's Cb, so the scale is a / 0.5 and the shear
// is a/2 * (b - CVShearB) / a, i.e. (b - CVShearB) / 2; Cr likewise with
// c and d. A tuning whose luma weights are not BT.601's is refused.
func Chroma(f *chromatix.File, o Options) ([]ChromaSet, error) {
	m, err := baseModule(f, "cv12")
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
	cct, err := selectRegion(root, o)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Name, err)
	}

	near := func(r *chromatix.Object, name string, want float64) error {
		if got := num(r, name); math.Abs(got-want) > cvTolerance {
			return fmt.Errorf("%s: %s is %g, not BT.601's %g", m.Name, name, got, want)
		}
		return nil
	}

	var out []ChromaSet
	for _, e := range cct {
		r := e.(*chromatix.Object)
		for _, c := range []struct {
			name string
			want float64
		}{
			{"r_to_y", 0.299}, {"g_to_y", 0.587}, {"b_to_y", 0.114},
		} {
			if err := near(r, c.name, c.want); err != nil {
				return nil, err
			}
		}
		trig := obj(r, "cct_trigger")
		s := ChromaSet{CT: representativeCT(num(trig, "start"), num(trig, "end"))}
		for i, k := range []struct {
			scale, shear string
			neutral      float64
		}{
			{"ap", "bp", isp.CVShearB}, {"am", "bm", isp.CVShearB},
			{"cp", "dp", isp.CVShearD}, {"cm", "dm", isp.CVShearD},
		} {
			s.Scale[i] = num(r, k.scale) / isp.CVScaleNeutral
			/* The tuning's float32s are BT.601's only to about 1e-8 */
			if h := (num(r, k.shear) - k.neutral) / 2; math.Abs(h) > 1e-6 {
				s.Shear[i] = h
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CT < out[j].CT })

	var sets []ChromaSet
	for _, s := range out {
		if len(sets) == 0 || sets[len(sets)-1].CT != s.CT {
			sets = append(sets, s)
		}
	}
	return sets, nil
}
