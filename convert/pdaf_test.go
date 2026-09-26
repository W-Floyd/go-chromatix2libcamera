package convert

import (
	"testing"

	"github.com/W-Floyd/go-chromatix2libcamera/calibration"
)

func TestPDAFFromCalibrationSwapsSides(t *testing.T) {
	p := PDAFFromCalibration(&calibration.PDAF{
		GainWidth: 2, GainHeight: 1, GainQ: 8,
		GainLeft:  []int{256, 512},
		GainRight: []int{768, 1024},
		DCCWidth:  1, DCCHeight: 1, DCCQ: 4,
		DCC: []int{250},
	})
	if p.GainLeft[0] != 3 || p.GainLeft[1] != 4 || p.GainRight[0] != 1 || p.GainRight[1] != 2 {
		t.Errorf("gains left %v right %v", p.GainLeft, p.GainRight)
	}
	if p.DCC[0] != 15.625 || p.Offset != nil {
		t.Errorf("dcc %v offset %v", p.DCC, p.Offset)
	}
}
