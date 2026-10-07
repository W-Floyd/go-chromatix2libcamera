package convert

import (
	"fmt"
	"sort"

	"github.com/W-Floyd/go-chromatix2libcamera/calibration"
	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// LSC is a module's lens shading correction: gain grids per colour
// temperature, their points spread from corner to corner of the image.
type LSC struct {
	Width, Height int
	Sets          []LSCSet
	// ByLux are the sets of every lux index region, if the module is
	// selected by lux index.
	ByLux []LSCLux
	// Calibrated is whether the gains include the unit's calibration.
	Calibrated bool
}

// LSCLux is the lens shading of one lux index region.
type LSCLux struct {
	Start, End float64
	Sets       []LSCSet
}

// LSCSet is the gains under one illuminant, per channel, row major.
// Chromatix has a table for each of the two greens; libcamera's debayer
// corrects one green, their mean.
type LSCSet struct {
	CT      int
	R, G, B []float64
}

// lscModules are the lens shading modules by preference.
var lscModules = []string{"lsc35", "lsc34"}

// LensShading extracts the lens shading gains of the selected region (the
// lens position nearest infinity, then the same regions as the CCMs). With
// a unit's calibration, each gain is scaled by the golden module's level
// over the unit's at its point, per channel, before the greens are
// averaged, as CamX's LSC34Interpolation does with one calibration table.
func LensShading(f *chromatix.File, o Options, cal *calibration.LSC) (*LSC, error) {
	var m chromatix.Symbol
	var err error
	for _, base := range lscModules {
		if m, err = baseModule(f, base); err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("no lens shading module")
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
	if num(root, "rolloff_enable") == 0 {
		return nil, fmt.Errorf("%s: lens shading is disabled", m.Name)
	}

	lens, err := pick(list(root, "core"), "lens_position", 0, "start", "end")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Name, err)
	}
	cct, err := selectFromDRC(list(lens, "lens_position_data"),
		str(root, "aec_exp_control") == "lux_idx", o)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Name, err)
	}

	floats := func(r *chromatix.Object, name string) []float64 {
		var out []float64
		for _, x := range list(r, name) {
			out = append(out, x.(float64))
		}
		return out
	}

	/* Per channel golden over unit, or nil for none */
	var fr, fgr, fgb, fb []float64
	if cal != nil {
		golden := list(root, "golden")
		if len(golden) == 0 {
			return nil, fmt.Errorf("%s: no golden module tables to calibrate against", m.Name)
		}
		g := golden[0].(*chromatix.Object)
		ratio := func(name string, unit []float64) []float64 {
			gt := floats(g, name)
			out := make([]float64, len(gt))
			for i := range gt {
				out[i] = 1
				if i < len(unit) && unit[i] != 0 {
					out[i] = gt[i] / unit[i]
				}
			}
			return out
		}
		fr, fgr = ratio("r_gain", cal.R), ratio("gr_gain", cal.Gr)
		fgb, fb = ratio("gb_gain", cal.Gb), ratio("b_gain", cal.B)
	}
	scale := func(v, factor []float64) []float64 {
		for i := range v {
			if i < len(factor) {
				v[i] *= factor[i]
			}
		}
		return v
	}

	setsOf := func(cct []chromatix.Value) []LSCSet {
		var all []LSCSet
		for _, e := range cct {
			r := e.(*chromatix.Object)
			trig := obj(r, "cct_trigger")
			s := LSCSet{
				CT: representativeCT(num(trig, "start"), num(trig, "end")),
				R:  scale(floats(r, "r_gain"), fr),
				B:  scale(floats(r, "b_gain"), fb),
			}
			gr, gb := scale(floats(r, "gr_gain"), fgr), scale(floats(r, "gb_gain"), fgb)
			for i := range gr {
				s.G = append(s.G, (gr[i]+gb[i])/2)
			}
			all = append(all, s)
		}
		sort.Slice(all, func(i, j int) bool { return all[i].CT < all[j].CT })

		/* Keep one set per colour temperature */
		var sets []LSCSet
		for _, s := range all {
			if len(sets) == 0 || sets[len(sets)-1].CT != s.CT {
				sets = append(sets, s)
			}
		}
		return sets
	}

	out := &LSC{Width: isp.LSCWidth, Height: isp.LSCHeight, Calibrated: cal != nil}
	out.Sets = setsOf(cct)

	/* The other lux regions, if the module has them */
	if regions, err := luxRegionsFrom(list(lens, "lens_position_data"),
		str(root, "aec_exp_control") == "lux_idx", o); err == nil {
		for _, r := range regions {
			out.ByLux = append(out.ByLux, LSCLux{Start: r.Start, End: r.End, Sets: setsOf(r.CCT)})
		}
	}

	return out, nil
}
