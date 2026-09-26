package convert

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
)

// LumaZone is one lux zone of the AEC luma target: from lux index Start to
// End (Qualcomm's lux index grows as the scene darkens), the mean luma
// target, on an 8-bit linear scale.
type LumaZone struct {
	Start, End float64
	Target     float64
}

// AECLumaTarget reads the AEC luma target of a tuned Chromatix file: the lux
// zones of the Metering module's luxLumaTargetZone. It is the mean
// luminance of the raw image, which libcamera's soft IPA can regulate on
// (the Agc meanTarget), and which the tuning's tone curve is made for.
func AECLumaTarget(f *chromatix.File) ([]LumaZone, error) {
	var metering *chromatix.Symbol
	for _, m := range f.Modules() {
		if m.Name == "Metering" {
			mm := m
			metering = &mm
			break
		}
	}
	if metering == nil {
		return nil, fmt.Errorf("no Metering module")
	}

	/*
	 * The module refers to its lists as 8-byte slots holding the count,
	 * then the symbol ID, of the list.
	 */
	data := f.Data(*metering)
	for off := 8; off+4 <= len(data); off += 8 {
		id := uint64(binary.LittleEndian.Uint32(data[off:]))
		s, ok := f.Symbol(id)
		if !ok || s.Name != "luxLumaTargetZone" {
			continue
		}

		count := int(binary.LittleEndian.Uint32(data[off-8:]))
		zones := f.Data(s)
		if count <= 0 || count*24 > len(zones) {
			return nil, fmt.Errorf("luxLumaTargetZone: %d zones in %d bytes", count, len(zones))
		}

		f32 := func(o int) float64 {
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(zones[o:])))
		}
		var out []LumaZone
		for i := 0; i < count; i++ {
			out = append(out, LumaZone{Start: f32(24 * i), End: f32(24*i + 8), Target: f32(24*i + 16)})
		}
		return out, nil
	}

	return nil, fmt.Errorf("Metering has no luxLumaTargetZone")
}

// HistZone is one lux zone of the AEC histogram luma target adjustment
// (Qualcomm's HistLumaTargetZoneType, aecMetering.xsd). Brightness
// references are on an 8-bit linear scale, ranges are fractions of the
// pixels counted from the bright end (saturation) or the dark end.
type HistZone struct {
	Start, End float64
	// MinTargetAdjRatio and MaxTargetAdjRatio bound how far the luma
	// target moves down (saturation prevention) and up (flat scenes).
	MinTargetAdjRatio, MaxTargetAdjRatio float64
	// SatRefLow and SatRefHigh: the saturation position, the brightness
	// of the brightest pixels, is pushed up towards SatRefLow and down
	// to SatRefHigh. DarkRefLow and DarkRefHigh likewise for the
	// darkest pixels.
	SatRefLow, SatRefHigh, DarkRefLow, DarkRefHigh float64
	// Pixel ranges, per R, G and B, as [start, end] fractions.
	SatPushupRange, SatPushdownRange, DarkRange [3][2]float64
	// MaxDRCGain caps the exposure reduction dynamic range compression
	// makes up for with tone mapping; MaxADRCGain the part of it taken
	// for highlights (safe versus short exposure), MaxDarkBoostGain the
	// part for shadows (long versus safe).
	MaxDRCGain, MaxADRCGain, MaxDarkBoostGain float64
}

// HistTarget is the histogram luma target adjustment of a tuned Chromatix
// file: whether it is enabled, whether ADRC and dark boost are, and the
// zones.
type HistTarget struct {
	Enable, ADRC, DarkBoost bool
	// Filter is the luma target filter factor: the previous value's
	// weight when the target changes.
	Filter float64
	Zones  []HistZone
}

// AECHistTarget reads the Metering module's histLumaTarget: its flags and
// its histLumaTargetZone list. The flags precede the zone list in the order
// of HistLumaTargetType: enableHistMetering, enableADRC,
// enableNonLinearTargetEstimate, enableDarkBoost,
// enableAntiBandingPrioOverADRC, antiBandingPrioTriggerRange,
// targetFilterFactor, then the zones' count and symbol.
func AECHistTarget(f *chromatix.File) (*HistTarget, error) {
	var metering *chromatix.Symbol
	for _, m := range f.Modules() {
		if m.Name == "Metering" {
			mm := m
			metering = &mm
			break
		}
	}
	if metering == nil {
		return nil, fmt.Errorf("no Metering module")
	}

	data := f.Data(*metering)
	u32 := func(b []byte, o int) uint32 { return binary.LittleEndian.Uint32(b[o:]) }
	for off := 8 * 8; off+4 <= len(data); off += 8 {
		s, ok := f.Symbol(uint64(u32(data, off)))
		if !ok || s.Name != "histLumaTargetZone" {
			continue
		}

		/* Seven flag and value slots, then the count, then the ID. */
		flags := off - 8*8
		t := &HistTarget{
			Enable:    u32(data, flags) != 0,
			ADRC:      u32(data, flags+8) != 0,
			DarkBoost: u32(data, flags+24) != 0,
			Filter:    float64(math.Float32frombits(u32(data, flags+48))),
		}

		count := int(u32(data, off-8))
		zones := f.Data(s)
		const slots = 29
		if count <= 0 || count*slots*8 > len(zones) {
			return nil, fmt.Errorf("histLumaTargetZone: %d zones in %d bytes", count, len(zones))
		}

		for i := 0; i < count; i++ {
			base := i * slots * 8
			v := func(slot int) float64 {
				return float64(math.Float32frombits(u32(zones, base+slot*8)))
			}
			z := HistZone{
				Start: v(0), End: v(1),
				MinTargetAdjRatio: v(2), MaxTargetAdjRatio: v(3),
				SatRefLow: v(4), SatRefHigh: v(5), DarkRefLow: v(6), DarkRefHigh: v(7),
				MaxDRCGain: v(26), MaxADRCGain: v(27), MaxDarkBoostGain: v(28),
			}
			for c := 0; c < 3; c++ {
				z.SatPushupRange[c] = [2]float64{v(8 + 2*c), v(9 + 2*c)}
				z.SatPushdownRange[c] = [2]float64{v(14 + 2*c), v(15 + 2*c)}
				z.DarkRange[c] = [2]float64{v(20 + 2*c), v(21 + 2*c)}
			}
			t.Zones = append(t.Zones, z)
		}
		return t, nil
	}

	return nil, fmt.Errorf("Metering has no histLumaTargetZone")
}
