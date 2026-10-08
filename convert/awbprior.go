package convert

import (
	"fmt"
	"sort"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// CTWeight is how plausible an illuminant of colour temperature CT is.
type CTWeight struct {
	CT, Weight float64
}

// LuxCTWeights is the illuminant weights of one lux index region.
type LuxCTWeights struct {
	Start, End float64
	Weights    []CTWeight
}

// ctWeightCurve turns weighted colour temperature ranges into a piecewise
// linear curve: a range holds its weight from its start to its end, a point
// is a point, and colour temperatures must strictly increase, as a libcamera
// Pwl needs (a later point at the same colour temperature is dropped).
func ctWeightCurve(ranges [][3]float64) []CTWeight {
	var pts []CTWeight
	for _, r := range ranges {
		pts = append(pts, CTWeight{r[0], r[2]})
		if r[1] > r[0] {
			pts = append(pts, CTWeight{r[1], r[2]})
		}
	}
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].CT < pts[j].CT })
	var out []CTWeight
	for _, p := range pts {
		if len(out) > 0 && p.CT <= out[len(out)-1].CT {
			continue
		}
		out = append(out, p)
	}
	return out
}

// AWBIlluminantPrior reads SFIlluWVV1, CamX AWB's weights on colour
// temperature by lux index, for every lux index region.
func AWBIlluminantPrior(f *chromatix.File) ([]LuxCTWeights, error) {
	var mod *chromatix.Symbol
	for _, m := range f.Modules() {
		if m.Name == "SFIlluWVV1" && (mod == nil || m.Group < mod.Group) {
			mm := m
			mod = &mm
		}
	}
	if mod == nil {
		return nil, fmt.Errorf("no SFIlluWVV1 module")
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
	if num(root, "enable") == 0 {
		return nil, fmt.Errorf("%s is disabled", mod.Name)
	}
	var out []LuxCTWeights
	for _, e := range list(root, "lux_weights") {
		r := e.(*chromatix.Object)
		tr := obj(r, "trigger")
		var ranges [][3]float64
		for _, c := range list(r, "cct_weights") {
			w := c.(*chromatix.Object)
			ct := obj(w, "cct")
			ranges = append(ranges, [3]float64{num(ct, "start"), num(ct, "end"), num(w, "weight")})
		}
		out = append(out, LuxCTWeights{num(tr, "start"), num(tr, "end"), ctWeightCurve(ranges)})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s has no lux regions", mod.Name)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out, nil
}
