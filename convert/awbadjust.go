package convert

import (
	"fmt"
	"sort"

	"github.com/W-Floyd/go-chromatix2libcamera/calibration"
	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// AdjustVertex is a vertex of the AWB gain adjustment mesh: a white point
// (R/G, B/G, as the statistics see it) and the factors CamX applies to the
// red and blue gains when the AWB decision lies there. Green is 1.
type AdjustVertex struct {
	RG, BG float64
	R, B   float64
}

// AdjustPoint is a gain adjustment factor pair at a colour temperature.
type AdjustPoint struct {
	CT   float64
	R, B float64
}

// GainAdjust is triglGAV1 for one lux index: the vertices and triangles of
// a mesh over R/G, B/G whose gains are interpolated across the triangle the
// AWB decision falls in, and gain factors by colour temperature. Both
// multiply the AWB gains; their effect is to keep some of warm light's
// warmth rather than neutralise it.
type GainAdjust struct {
	Vertices  []AdjustVertex
	Triangles [][3]int
	CCT       []AdjustPoint
}

// triggerRGB interpolates a list of awbTriggerDataRGBType at x: the value of
// the range containing x, a linear blend in the gap between two ranges, the
// nearest range's beyond them all. It returns the red and blue factors
// relative to green.
func triggerRGB(list []chromatix.Value, x float64) (r, b float64) {
	type entry struct{ start, end, r, b float64 }
	var es []entry
	for _, e := range list {
		o := e.(*chromatix.Object)
		t := obj(o, "trigger")
		g := num(o, "green")
		if g == 0 {
			g = 1
		}
		es = append(es, entry{num(t, "start"), num(t, "end"), num(o, "red") / g, num(o, "blue") / g})
	}
	if len(es) == 0 {
		return 1, 1
	}
	sort.Slice(es, func(i, j int) bool { return es[i].start < es[j].start })
	if x <= es[0].end {
		return es[0].r, es[0].b
	}
	for i := 0; i+1 < len(es); i++ {
		a, n := es[i], es[i+1]
		if x <= a.end {
			return a.r, a.b
		}
		if x < n.start {
			t := (x - a.end) / (n.start - a.end)
			return a.r + (n.r-a.r)*t, a.b + (n.b-a.b)*t
		}
	}
	last := es[len(es)-1]
	return last.r, last.b
}

// AWBGainAdjust reads triglGAV1 at the options' lux index. The mesh's
// vertices are white points of the golden module, like refPtV1's, and are
// scaled to this unit by cal when given.
func AWBGainAdjust(f *chromatix.File, o Options, cal *calibration.AWB) (*GainAdjust, error) {
	var mod *chromatix.Symbol
	for _, m := range f.Modules() {
		if m.Name == "triglGAV1" && (mod == nil || m.Group < mod.Group) {
			mm := m
			mod = &mm
		}
	}
	if mod == nil {
		return nil, fmt.Errorf("no triglGAV1 module")
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

	rgScale, bgScale := 1.0, 1.0
	if cal != nil {
		rgScale, bgScale = cal.Factors()
	}

	out := &GainAdjust{}
	for _, e := range list(root, "vertices") {
		vx := e.(*chromatix.Object)
		p := obj(vx, "point")
		r, b := triggerRGB(list(vx, "gain_adj"), o.LuxIdx)
		out.Vertices = append(out.Vertices, AdjustVertex{
			RG: num(p, "rg") * rgScale, BG: num(p, "bg") * bgScale, R: r, B: b,
		})
	}
	for _, e := range list(root, "triangles") {
		idx := list(e.(*chromatix.Object), "vertex_idx")
		var tri [3]int
		for i := range tri {
			tri[i] = int(int64Of(idx[i]))
			if tri[i] < 0 || tri[i] >= len(out.Vertices) {
				return nil, fmt.Errorf("%s: triangle vertex %d out of range", mod.Name, tri[i])
			}
		}
		out.Triangles = append(out.Triangles, tri)
	}

	/* By colour temperature: the lux entry containing the lux index, or nearest */
	var cct *chromatix.Object
	bestDist := 0.0
	for _, e := range list(root, "cct_adj") {
		c := e.(*chromatix.Object)
		tr := obj(c, "trigger")
		d := 0.0
		if s, en := num(tr, "start"), num(tr, "end"); o.LuxIdx < s {
			d = s - o.LuxIdx
		} else if o.LuxIdx > en {
			d = o.LuxIdx - en
		}
		if cct == nil || d < bestDist {
			cct, bestDist = c, d
		}
	}
	if cct != nil {
		for _, e := range list(cct, "data") {
			d := e.(*chromatix.Object)
			tr := obj(d, "trigger")
			g := num(d, "green")
			if g == 0 {
				g = 1
			}
			r, b := num(d, "red")/g, num(d, "blue")/g
			out.CCT = append(out.CCT, AdjustPoint{num(tr, "start"), r, b}, AdjustPoint{num(tr, "end"), r, b})
		}
		sort.SliceStable(out.CCT, func(i, j int) bool { return out.CCT[i].CT < out.CCT[j].CT })
		/* Strictly increasing, as a libcamera Pwl needs */
		var pts []AdjustPoint
		for _, p := range out.CCT {
			if len(pts) > 0 && p.CT <= pts[len(pts)-1].CT {
				continue
			}
			pts = append(pts, p)
		}
		out.CCT = pts
	}
	return out, nil
}
