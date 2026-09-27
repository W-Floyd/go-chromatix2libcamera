package convert

import (
	"fmt"
	"math"
	"sort"

	"github.com/W-Floyd/go-chromatix2libcamera/calibration"
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

// MinTransverse is the least band either side of the locus: libipa's
// AwbBayes default for transversePos and transverseNeg.
const MinTransverse = 0.01

// Transverse is how far real illuminants lie off the locus, in R/G, B/G:
// the furthest of the off-locus points away from the origin (pos, towards
// purple) and towards it (neg, towards green), each at least
// MinTransverse. The soft IPA keeps grey world within that band.
func Transverse(locus, other []WhitePoint) (pos, neg float64) {
	pos, neg = MinTransverse, MinTransverse
	for _, w := range other {
		best := math.Inf(1)
		var side, dist float64
		for i := 0; i+1 < len(locus); i++ {
			p, q := locus[i], locus[i+1]
			dx, dy := q.RG-p.RG, q.BG-p.BG
			vx, vy := w.RG-p.RG, w.BG-p.BG
			t := math.Max(0, math.Min(1, (vx*dx+vy*dy)/(dx*dx+dy*dy)))
			ex, ey := vx-dx*t, vy-dy*t
			d := math.Hypot(ex, ey)
			if d >= best {
				continue
			}
			best = d
			/* The segment's normal away from the origin */
			nx, ny := dy, -dx
			if nx*p.RG+ny*p.BG < 0 {
				nx, ny = -nx, -ny
			}
			side, dist = ex*nx+ey*ny, d
		}
		if side >= 0 {
			pos = math.Max(pos, dist)
		} else {
			neg = math.Max(neg, dist)
		}
	}
	return pos, neg
}

// CalibrateWhitePoints moves the golden module's white points to this
// unit's, as CamX does with the EEPROM's white balance calibration: R/G and
// B/G scaled by unit over golden. The tuning's sensorCalV1 could give
// factors per colour temperature; the calibration here has one, at one
// light, which applies to every point.
func CalibrateWhitePoints(points []WhitePoint, cal *calibration.AWB) {
	rg, bg := cal.Factors()
	for i := range points {
		points[i].RG *= rg
		points[i].BG *= bg
	}
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
