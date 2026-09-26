package convert

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/W-Floyd/go-chromatix2libcamera/calibration"
	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
)

// PDAF is a module's PDAF calibration in real units, for the Af algorithm.
type PDAF struct {
	// Source says where it came from, for a comment in the tuning file.
	Source string
	// GainWidth x GainHeight grid points, corner to corner; GainLeft and
	// GainRight are gains (1.0 = none) for the PD pixels that see the
	// left and the right half of the pupil, as libcamera's PDAF layout
	// names them.
	GainWidth, GainHeight int
	GainLeft, GainRight   []float64
	// DCCWidth x DCCHeight regions; DCC is actuator codes per image pixel
	// of phase.
	DCCWidth, DCCHeight int
	DCC                 []float64
	// Offset, if set, is added to the phase, in image pixels.
	Offset *float64
}

// PDAFFromCalibration converts EEPROM PDAF calibration to real units.
//
// The EEPROM names its gain maps the other way round from libcamera's
// layout: its left map is for the pixels libcamera's layout calls right
// (checked on the S5KJN1: only that pairing flattens the left/right ratio
// across the frame), so they are swapped here.
func PDAFFromCalibration(p *calibration.PDAF) *PDAF {
	scale := func(v []int, q int) []float64 {
		out := make([]float64, len(v))
		for i, x := range v {
			out[i] = float64(x) / float64(int(1)<<q)
		}
		return out
	}
	return &PDAF{
		Source: fmt.Sprintf("EEPROM PDAF calibration v%d, gain mode %#x, DCC mode %#x, orientation %#x",
			p.Version, p.GainMode, p.DCCMode, p.Orientation),
		GainWidth:  p.GainWidth,
		GainHeight: p.GainHeight,
		GainLeft:   scale(p.GainRight, p.GainQ),
		GainRight:  scale(p.GainLeft, p.GainQ),
		DCCWidth:   p.DCCWidth,
		DCCHeight:  p.DCCHeight,
		DCC:        scale(p.DCC, p.DCCQ),
	}
}

// PDAFOffsetFromModule reads the phase offset correction of the sensor
// module file (PDOffsetCorrection, a float per PD mode, in image pixels as
// the DCC is). ok is false if the file has none, or if the modes disagree,
// as there is no telling which mode libcamera runs.
func PDAFOffsetFromModule(f *chromatix.File) (offset float64, ok bool) {
	for _, s := range f.Symbols {
		if s.Name != "PDOffsetCorrection" || s.Size < 4 {
			continue
		}
		v := float64(math.Float32frombits(binary.LittleEndian.Uint32(f.Data(s))))
		if ok && v != offset {
			return 0, false
		}
		offset, ok = v, true
	}
	return offset, ok
}
