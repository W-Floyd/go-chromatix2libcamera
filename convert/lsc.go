package convert

import (
	"fmt"
	"sort"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// LSC is a module's lens shading correction: gain grids per colour
// temperature, their points spread from corner to corner of the image.
type LSC struct {
	Width, Height int
	Sets          []LSCSet
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
// lens position nearest infinity, then the same regions as the CCMs).
func LensShading(f *chromatix.File, o Options) (*LSC, error) {
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

	out := &LSC{Width: isp.LSCWidth, Height: isp.LSCHeight}
	for _, e := range cct {
		r := e.(*chromatix.Object)
		trig := obj(r, "cct_trigger")
		s := LSCSet{
			CT: representativeCT(num(trig, "start"), num(trig, "end")),
			R:  floats(r, "r_gain"),
			B:  floats(r, "b_gain"),
		}
		gr, gb := floats(r, "gr_gain"), floats(r, "gb_gain")
		for i := range gr {
			s.G = append(s.G, (gr[i]+gb[i])/2)
		}
		out.Sets = append(out.Sets, s)
	}
	sort.Slice(out.Sets, func(i, j int) bool { return out.Sets[i].CT < out.Sets[j].CT })

	/* Keep one set per colour temperature */
	var sets []LSCSet
	for _, s := range out.Sets {
		if len(sets) == 0 || sets[len(sets)-1].CT != s.CT {
			sets = append(sets, s)
		}
	}
	out.Sets = sets

	return out, nil
}
