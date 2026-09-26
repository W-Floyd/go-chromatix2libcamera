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
