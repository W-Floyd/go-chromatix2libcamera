package convert

import (
	"fmt"
	"sort"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// WhitePoint is a grey surface's R/G and B/G, as the AWB statistics see it,
// under the illuminant of colour temperature CT.
type WhitePoint struct {
	CT     int
	RG, BG float64
	// Name is the CamX illuminant the point was tuned for.
	Name string
}

// Gains are the white balance gains that make the point grey, red and
// blue relative to green.
func (p WhitePoint) Gains() (r, b float64) {
	return 1 / p.RG, 1 / p.BG
}

// WhitePoints reads the AWB reference points (refPtV1) of a tuned Chromatix
// file and returns those on CamX's daylight locus, by increasing colour
// temperature, and the others, which have none (the fluorescent points).
func WhitePoints(f *chromatix.File) (locus, other []WhitePoint, err error) {
	for _, m := range f.Modules() {
		if m.Name != "refPtV1" {
			continue
		}
		t, ok := isp.Schema(m)
		if !ok {
			return nil, nil, fmt.Errorf("no schema for %s %s", m.Name, m.VersionString())
		}
		v, err := f.Decode(m, t)
		if err != nil {
			return nil, nil, err
		}
		o := v.(*chromatix.Object)
		if num(o, "enable") == 0 {
			return nil, nil, fmt.Errorf("%s is disabled", m.Name)
		}
		for i, e := range list(o, "ref_pt") {
			p := e.(*chromatix.Object)
			wp := WhitePoint{RG: num(p, "rg"), BG: num(p, "bg")}
			if wp.RG <= 0 || wp.BG <= 0 {
				continue
			}
			if i < len(isp.RefPtIlluminants) {
				wp.Name = isp.RefPtIlluminants[i]
			}
			if ct, ok := isp.RefPtCT[i]; ok {
				wp.CT = ct
				locus = append(locus, wp)
			} else {
				other = append(other, wp)
			}
		}
		if len(locus) < 2 {
			return nil, nil, fmt.Errorf("%s: %d points on the locus", m.Name, len(locus))
		}
		sort.Slice(locus, func(i, j int) bool { return locus[i].CT < locus[j].CT })
		return locus, other, nil
	}
	return nil, nil, fmt.Errorf("no refPtV1 module")
}
