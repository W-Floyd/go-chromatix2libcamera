// Package convert turns decoded Chromatix modules into libcamera tuning
// data for the software ISP (the "simple" IPA).
package convert

import (
	"fmt"
	"math"
	"math/bits"
	"sort"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// Options select which of a module's trigger regions to convert, since
// libcamera's software ISP indexes its tables by colour temperature only.
type Options struct {
	// LuxIdx picks the AEC region containing this Qualcomm lux index
	// (higher is darker; ~250 is ordinary indoor light).
	LuxIdx float64
	// DRCGain picks the DRC gain region containing this gain; 1 is no
	// dynamic range compression.
	DRCGain float64
}

// DefaultOptions are ordinary indoor light, no flash, no DRC.
var DefaultOptions = Options{LuxIdx: 250, DRCGain: 1}

// CCM is one colour correction matrix at a colour temperature, in
// libcamera's row-major RGB order.
type CCM struct {
	CT     int
	Matrix [9]float64
}

// baseModule returns the module of that base name from the default OPE
// or IPE group: groups beyond those hold per-usecase overrides.
func baseModule(f *chromatix.File, base string) (chromatix.Symbol, error) {
	for _, suffix := range []string{"_ope", "_ipe", "_bps", "_ife", "_tfe"} {
		var best *chromatix.Symbol
		for _, m := range f.Modules() {
			if m.Name != base+suffix {
				continue
			}
			if best == nil || m.Group < best.Group {
				mm := m
				best = &mm
			}
		}
		if best != nil {
			return *best, nil
		}
	}
	return chromatix.Symbol{}, fmt.Errorf("no %s module", base)
}

// CCMs extracts the colour correction matrices for the selected region.
func CCMs(f *chromatix.File, o Options) ([]CCM, error) {
	m, err := baseModule(f, "cc13")
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

	cct, err := selectRegion(v.(*chromatix.Object), o)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Name, err)
	}

	var out []CCM
	for _, e := range cct {
		r := e.(*chromatix.Object)
		trig := obj(r, "cct_trigger")
		var c CCM
		c.CT = representativeCT(num(trig, "start"), num(trig, "end"))
		for i, x := range list(r, "c") {
			c.Matrix[i] = x.(float64)
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CT < out[j].CT })
	return dedupeCT(out), nil
}

// Point is a point of a tone curve, both coordinates in [0, 1].
type Point struct{ X, Y float64 }

// ToneCurve returns the gamma15 output curve for the selected region, as
// libcamera's software ISP "toneCurve": linear light in, encoded value out.
// Chromatix tables are per channel; the green one is used, since libcamera
// applies one curve to all three (and tuned files usually have them equal).
func ToneCurve(f *chromatix.File, o Options) ([]Point, error) {
	m, err := baseModule(f, "gamma15")
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
	cct, err := selectRegion(v.(*chromatix.Object), o)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Name, err)
	}

	// Colour temperature: the region containing daylight, or the first.
	region := cct[0].(*chromatix.Object)
	for _, e := range cct {
		r := e.(*chromatix.Object)
		t := obj(r, "cct_trigger")
		if num(t, "start") <= 5000 && num(t, "end") >= 5000 {
			region = r
			break
		}
	}

	var table []chromatix.Value
	for _, e := range list(region, "cct_data") {
		ch := e.(*chromatix.Object)
		if c := str(ch, "channel"); c == "G" || c == "RGB" || table == nil {
			table = list(ch, "table")
		}
	}
	if len(table) != isp.GammaTableLen {
		return nil, fmt.Errorf("%s: gamma table has %d entries", m.Name, len(table))
	}

	/*
	 * The output codes' full scale: gamma_1_5_0.xsd gives the range as
	 * [0,1023], but tuned files hold 8-bit codes too (the S5KJNS module's
	 * top out at 255), so take the smallest 2^n - 1 the table fits in.
	 */
	maxCode := 0.0
	for _, y := range table {
		maxCode = math.Max(maxCode, y.(float64))
	}
	fullScale := float64(uint64(1)<<bits.Len64(uint64(maxCode)) - 1)
	if fullScale <= 0 {
		return nil, fmt.Errorf("%s: gamma table is all zero", m.Name)
	}

	last := float64(len(table) - 1)
	pts := make([]Point, len(table))
	for i, y := range table {
		pts[i] = Point{X: float64(i) / last, Y: y.(float64) / fullScale}
	}
	return pts, nil
}

// blackLevelModule decodes the front end's demuxblklevel14 module.
func blackLevelModule(f *chromatix.File) (*chromatix.Object, error) {
	var mod *chromatix.Symbol
	for _, m := range f.Modules() {
		if m.Name == "demuxblklevel14_tfe" && (mod == nil || m.Group < mod.Group) {
			mm := m
			mod = &mm
		}
	}
	if mod == nil {
		return nil, fmt.Errorf("no demuxblklevel14_tfe module")
	}
	t, ok := isp.Schema(*mod)
	if !ok {
		return nil, fmt.Errorf("no schema for %s %s", mod.Name, mod.VersionString())
	}
	v, err := f.Decode(*mod, t)
	if err != nil {
		return nil, err
	}
	return v.(*chromatix.Object), nil
}

// blackLevel16 is a colour temperature region's black level on libcamera's
// 16-bit scale, averaged over the four Bayer channels. The black level
// does not depend on colour temperature; the first region is taken.
func blackLevel16(cct []chromatix.Value) float64 {
	sum := 0.0
	levels := list(cct[0].(*chromatix.Object), "black_level")
	for _, l := range levels {
		sum += l.(float64)
	}
	return sum / float64(len(levels)) * float64(uint(1)<<(16-isp.DemuxBlkLevelBits))
}

// BlackLevel returns the front-end black level for the selected region, on
// libcamera's 16-bit scale, averaged over the four Bayer channels.
func BlackLevel(f *chromatix.File, o Options) (int, error) {
	root, err := blackLevelModule(f)
	if err != nil {
		return 0, err
	}
	cct, err := selectRegion(root, o)
	if err != nil {
		return 0, fmt.Errorf("demuxblklevel14_tfe: %w", err)
	}
	return int(blackLevel16(cct) + 0.5), nil
}

// GainLevel is a black level (libcamera's 16-bit scale) at a sensor gain.
type GainLevel struct {
	Gain, Level float64
}

// BlackLevelByGain returns the front-end black level as a function of
// sensor gain, when the tuning selects it by gain: each gain region's
// level at its start and end, so that the level is flat within a region
// and linear between regions, as CamX interpolates. Nil when the tuning
// selects it by lux index or has one gain region.
func BlackLevelByGain(f *chromatix.File, o Options) ([]GainLevel, error) {
	root, err := blackLevelModule(f)
	if err != nil {
		return nil, err
	}
	if str(root, "aec_exp_control") != "gain" {
		return nil, nil
	}

	drc, err := pick(list(root, "core"), "drc_gain", o.DRCGain, "start", "end")
	if err != nil {
		return nil, err
	}
	hdr := list(drc, "drc_gain_data")[0].(*chromatix.Object)
	var led *chromatix.Object
	for _, e := range list(hdr, "hdr_aec_data") {
		if l := e.(*chromatix.Object); int64Of(get(l, "led_idx_trigger")) == 0 {
			led = l
			break
		}
	}
	if led == nil {
		return nil, fmt.Errorf("demuxblklevel14_tfe: no region without flash")
	}

	var out []GainLevel
	for _, e := range list(led, "led_idx_data") {
		r := e.(*chromatix.Object)
		t := obj(r, "aec_trigger")
		level := blackLevel16(list(r, "aec_data"))
		out = append(out, GainLevel{num(t, "gain_start"), level}, GainLevel{num(t, "gain_end"), level})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Gain < out[j].Gain })

	/* Keep the gains strictly increasing, as a libcamera Pwl needs */
	var pwl []GainLevel
	for _, p := range out {
		if len(pwl) > 0 && p.Gain <= pwl[len(pwl)-1].Gain {
			continue
		}
		pwl = append(pwl, p)
	}
	if len(pwl) < 2 {
		return nil, nil
	}
	return pwl, nil
}

// selectRegion walks a module's DRC gain -> HDR -> LED -> AEC trigger tree
// and returns the colour temperature level of the selected region.
func selectRegion(root *chromatix.Object, o Options) ([]chromatix.Value, error) {
	return selectFromDRC(list(root, "core"), str(root, "aec_exp_control") == "lux_idx", o)
}

// selectFromDRC walks the tree from its DRC gain level down, for modules
// whose tree has levels above it (lens shading starts at lens position).
func selectFromDRC(core []chromatix.Value, aecByLux bool, o Options) ([]chromatix.Value, error) {
	drc, err := pick(core, "drc_gain", o.DRCGain, "start", "end")
	if err != nil {
		return nil, err
	}
	// HDR ratio: the first region is the non-HDR one.
	hdr := list(drc, "drc_gain_data")[0].(*chromatix.Object)
	var led *chromatix.Object
	for _, e := range list(hdr, "hdr_aec_data") {
		if l := e.(*chromatix.Object); int64Of(get(l, "led_idx_trigger")) == 0 {
			led = l
			break
		}
	}
	if led == nil {
		return nil, fmt.Errorf("no region without flash")
	}
	var aec *chromatix.Object
	if aecByLux {
		aec, err = pick(list(led, "led_idx_data"), "aec", o.LuxIdx, "lux_idx_start", "lux_idx_end")
	} else {
		// Gain-indexed: gain 1 is the brightest, least noisy case.
		aec, err = pick(list(led, "led_idx_data"), "aec", 1, "gain_start", "gain_end")
	}
	if err != nil {
		return nil, err
	}
	return list(aec, "aec_data"), nil
}

// pick returns the region whose [start, end] trigger contains x or, when x
// falls in the interpolation gap between two regions or beyond them all,
// the region nearest to it.
func pick(regions []chromatix.Value, level string, x float64, startName, endName string) (*chromatix.Object, error) {
	if len(regions) == 0 {
		return nil, fmt.Errorf("no %s regions", level)
	}
	var best *chromatix.Object
	bestDist := 0.0
	for _, e := range regions {
		r := e.(*chromatix.Object)
		t := obj(r, level+"_trigger")
		start, end := num(t, startName), num(t, endName)
		var d float64
		switch {
		case x < start:
			d = start - x
		case x > end:
			d = x - end
		}
		if best == nil || d < bestDist {
			best, bestDist = r, d
		}
	}
	return best, nil
}

// representativeCT picks one colour temperature for a CCT trigger range:
// the finite end of an open-ended range, else the midpoint.
func representativeCT(start, end float64) int {
	switch {
	case start <= 1 && end >= 10000:
		return 5000
	case start <= 1:
		return int(end)
	case end >= 10000:
		return int(start)
	default:
		return int((start + end) / 2)
	}
}

func dedupeCT(in []CCM) []CCM {
	var out []CCM
	for _, c := range in {
		if len(out) > 0 && out[len(out)-1].CT == c.CT {
			continue
		}
		out = append(out, c)
	}
	return out
}

func get(o *chromatix.Object, name string) chromatix.Value {
	v, ok := o.Get(name)
	if !ok {
		panic("schema has no field " + name)
	}
	return v
}

func obj(o *chromatix.Object, name string) *chromatix.Object {
	return get(o, name).(*chromatix.Object)
}

func list(o *chromatix.Object, name string) []chromatix.Value {
	return get(o, name).([]chromatix.Value)
}

func num(o *chromatix.Object, name string) float64 {
	switch v := get(o, name).(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	case uint64:
		return float64(v)
	}
	panic("field " + name + " is not a number")
}

func str(o *chromatix.Object, name string) string {
	return get(o, name).(string)
}

func int64Of(v chromatix.Value) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case uint64:
		return int64(x)
	}
	return -1
}
