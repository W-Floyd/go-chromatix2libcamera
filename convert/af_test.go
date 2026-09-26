package convert

import (
	"math"
	"testing"

	"github.com/W-Floyd/go-chromatix2libcamera/calibration"
)

func TestBuildAFMapFromPoints(t *testing.T) {
	a := &Actuator{Name: "vcm", MaxCode: 1023, InitialCode: 40}
	cal := &calibration.AF{
		InfinityCode: 525, MacroCode: 753, MacroMargin: 0.2,
		Points: []calibration.AFPoint{
			{DistanceMM: 300, Code: 597}, {DistanceMM: 500, Code: 567},
			{DistanceMM: 750, Code: 552}, {DistanceMM: 10000, Code: 525},
		},
	}
	m := BuildAFMap(a, cal, DefaultMacroDioptres)

	for i := 1; i < len(m.Points); i++ {
		if m.Points[i].Dioptres <= m.Points[i-1].Dioptres || m.Points[i].Code <= m.Points[i-1].Code {
			t.Fatalf("map not strictly increasing: %+v", m.Points)
		}
	}
	first, last := m.Points[0], m.Points[len(m.Points)-1]
	if first.Dioptres != 0 || math.Abs(float64(first.Code-523)) > 2 {
		t.Errorf("infinity end %+v", first)
	}
	// Macro plus 20% of the calibrated range.
	if last.Code != 799 {
		t.Errorf("macro end with margin %+v, want code 799", last)
	}
}

func TestBuildAFMapWithoutCalibration(t *testing.T) {
	a := &Actuator{Name: "vcm", MaxCode: 1023, InitialCode: 40}
	m := BuildAFMap(a, nil, 8)
	if len(m.Points) != 2 || m.Points[0] != (MapPoint{0, 40}) || m.Points[1] != (MapPoint{8, 1023}) {
		t.Errorf("map %+v", m.Points)
	}
}
