package convert

import (
	"fmt"

	"github.com/W-Floyd/go-chromatix2libcamera/calibration"
)

// PDAF is a module's PDAF calibration in real units, for the Af algorithm.
type PDAF struct {
	// Source says where it came from, for a comment in the tuning file.
	Source string
	// GainWidth x GainHeight grid points, corner to corner; GainLeft and
	// GainRight are gains (1.0 = none), named as the EEPROM names them.
	GainWidth, GainHeight int
	GainLeft, GainRight   []float64
	// DCCWidth x DCCHeight regions; DCC is actuator codes per pixel of
	// phase.
	DCCWidth, DCCHeight int
	DCC                 []float64
}

// PDAFFromCalibration converts EEPROM PDAF calibration to real units.
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
		GainLeft:   scale(p.GainLeft, p.GainQ),
		GainRight:  scale(p.GainRight, p.GainQ),
		DCCWidth:   p.DCCWidth,
		DCCHeight:  p.DCCHeight,
		DCC:        scale(p.DCC, p.DCCQ),
	}
}
