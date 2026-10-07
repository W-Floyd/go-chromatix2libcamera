package convert

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
)

// KneeEntry is one knee of the AEC's exposure table (expKneeEntries): the
// gain and the exposure time (ns) the table reaches at the exposure index.
// CamX's lux index is the exposure index of the exposure that meets the
// luma target, as its log line "Use safe exposure index as approximation for
// lux index" says, so the knees give the lux index of an exposure.
type KneeEntry struct {
	Gain      float64
	ExpTimeNs float64
	Index     float64
}

// AECKnees reads the first exposure table of the file, which is the main
// AEC group's (the later ones are other use cases'): expTables holds its
// tables, each a scale, the count of knees, the symbol of the knee entries
// and a sensitivity correction factor, 8 bytes to a slot. A knee has the
// gain, the exposure time in ns, a priority and the exposure index.
func AECKnees(f *chromatix.File) ([]KneeEntry, error) {
	u32 := func(b []byte, o int) uint32 { return binary.LittleEndian.Uint32(b[o:]) }
	for _, s := range f.Symbols {
		if s.Name != "expTables" {
			continue
		}

		tables := f.Data(s)
		if len(tables) < 32 {
			return nil, fmt.Errorf("expTables: %d bytes", len(tables))
		}
		count := int(u32(tables, 8))
		k, ok := f.Symbol(uint64(u32(tables, 16)))
		if !ok || k.Name != "expKneeEntries" {
			return nil, fmt.Errorf("expTables: no knee entries")
		}
		raw := f.Data(k)
		if count <= 0 || count*32 > len(raw) {
			return nil, fmt.Errorf("expKneeEntries: %d knees in %d bytes", count, len(raw))
		}

		f32 := func(o int) float64 {
			return float64(math.Float32frombits(u32(raw, o)))
		}
		var out []KneeEntry
		for i := 0; i < count; i++ {
			b := i * 32
			out = append(out, KneeEntry{
				Gain:      f32(b),
				ExpTimeNs: float64(u32(raw, b+8)),
				Index:     f32(b + 24),
			})
		}
		return out, nil
	}
	return nil, fmt.Errorf("no expTables")
}

// LuxFromKnees fits the lux index to the knees: it is linear in the log of
// the exposure's sensitivity, gain times exposure time, with the index
// steps per stop of it and the sensitivity (in us times gain) of lux index
// 250 that Agc's lux model takes. On the S5KJNS the knees fit to within
// 0.01 index (a step is 3% of sensitivity: 23.45 per stop).
func LuxFromKnees(knees []KneeEntry) (*LuxModel, error) {
	if len(knees) < 2 {
		return nil, fmt.Errorf("%d knees", len(knees))
	}

	var sx, sy, sxx, sxy float64
	n := float64(len(knees))
	for _, k := range knees {
		if k.Gain <= 0 || k.ExpTimeNs <= 0 {
			return nil, fmt.Errorf("knee with gain %g and exposure time %g ns", k.Gain, k.ExpTimeNs)
		}
		x := math.Log2(k.Gain * k.ExpTimeNs / 1000)
		sx += x
		sy += k.Index
		sxx += x * x
		sxy += x * k.Index
	}
	perStop := (n*sxy - sx*sy) / (n*sxx - sx*sx)
	offset := (sy - perStop*sx) / n
	if perStop <= 0 {
		return nil, fmt.Errorf("knees do not rise with sensitivity")
	}

	for _, k := range knees {
		x := math.Log2(k.Gain * k.ExpTimeNs / 1000)
		if d := math.Abs(offset + perStop*x - k.Index); d > 0.5 {
			return nil, fmt.Errorf("knee at index %g is %g off the fit", k.Index, d)
		}
	}

	return &LuxModel{
		RefProduct: math.Exp2((250 - offset) / perStop),
		PerStop:    perStop,
	}, nil
}
