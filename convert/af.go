package convert

import (
	"fmt"
	"math"
	"sort"

	"github.com/W-Floyd/go-chromatix2libcamera/calibration"
	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// DefaultMacroDioptres is where the macro end is assumed to focus when the
// calibration gives no distances: 10 dioptres is 10 cm, a typical phone
// module's minimum focus distance.
const DefaultMacroDioptres = 10.0

// MapPoint is one point of libcamera's Af map.
type MapPoint struct {
	Dioptres float64
	Code     int
}

// AFMap is the Af map for a module, with a note on where it came from.
type AFMap struct {
	Actuator string
	Source   string
	Points   []MapPoint
}

// Actuator is what the sensor module says about the focus actuator.
type Actuator struct {
	Name    string
	MaxCode int
	// InitialCode is the tuned infinity code. The tuned step table beyond
	// it is a placeholder: CamX replaces it with the EEPROM calibration.
	InitialCode int
}

// ActuatorFromFile reads the actuator description from a sensor module.
func ActuatorFromFile(f *chromatix.File) (*Actuator, error) {
	for _, m := range f.Modules() {
		if m.Name != "actuatorDriver" {
			continue
		}
		t, _ := isp.Schema(m)
		v, err := f.Decode(m, t)
		if err != nil {
			return nil, err
		}
		o := v.(*chromatix.Object)
		return &Actuator{
			Name:        str(o, "actuatorName"),
			MaxCode:     1<<int(num(o, "dataBitWidth")) - 1,
			InitialCode: int(num(o, "initialCode")),
		}, nil
	}
	return nil, fmt.Errorf("no actuatorDriver")
}

// BuildAFMap derives the Af map. With per-unit calibration it uses the
// calibration points (actuator codes are close to linear in dioptres, so
// a least-squares line through them also places the macro code) or the
// infinity and macro codes with macroDioptres; it then extends the macro
// end by the module's margin, as CamX does, within the DAC's range.
// Without calibration it spans the whole DAC from the tuned initial code.
func BuildAFMap(a *Actuator, cal *calibration.AF, macroDioptres float64) AFMap {
	m := AFMap{Actuator: a.Name}

	if cal == nil {
		m.Source = fmt.Sprintf("no calibration: codes %d-%d over 0-%g dioptres (-macro-dioptres)",
			a.InitialCode, a.MaxCode, macroDioptres)
		m.Points = []MapPoint{{0, a.InitialCode}, {macroDioptres, a.MaxCode}}
		return m
	}

	// code = slope * dioptres + offset
	var slope, offset float64
	if len(cal.Points) >= 2 {
		slope, offset = fitLine(cal.Points)
		m.Source = fmt.Sprintf("EEPROM calibration, %d points; macro code %d fitted to %.1f dioptres",
			len(cal.Points), cal.MacroCode, (float64(cal.MacroCode)-offset)/slope)
	} else {
		offset = float64(cal.InfinityCode)
		slope = float64(cal.MacroCode-cal.InfinityCode) / macroDioptres
		m.Source = fmt.Sprintf("EEPROM infinity/macro codes %d/%d, macro at %g dioptres (-macro-dioptres)",
			cal.InfinityCode, cal.MacroCode, macroDioptres)
	}
	if slope <= 0 {
		m.Source += "; calibration not increasing, using the endpoints"
		slope = float64(cal.MacroCode-cal.InfinityCode) / macroDioptres
		offset = float64(cal.InfinityCode)
	}
	at := func(code float64) float64 { return (code - offset) / slope }

	pts := []MapPoint{{0, int(math.Round(offset))}}
	for _, p := range cal.Points {
		if p.DistanceMM > 0 {
			pts = append(pts, MapPoint{1000 / float64(p.DistanceMM), p.Code})
		}
	}
	pts = append(pts, MapPoint{at(float64(cal.MacroCode)), cal.MacroCode})
	if cal.MacroMargin > 0 {
		code := float64(cal.MacroCode) + cal.MacroMargin*float64(cal.MacroCode-cal.InfinityCode)
		code = math.Min(code, float64(a.MaxCode))
		pts = append(pts, MapPoint{at(code), int(math.Round(code))})
	}

	sort.Slice(pts, func(i, j int) bool { return pts[i].Dioptres < pts[j].Dioptres })
	for _, p := range pts {
		if len(m.Points) > 0 {
			last := m.Points[len(m.Points)-1]
			if p.Dioptres <= last.Dioptres+1e-3 || p.Code <= last.Code {
				continue // keep the map strictly increasing
			}
		}
		m.Points = append(m.Points, p)
	}
	return m
}

// fitLine fits code = slope * dioptres + offset by least squares.
func fitLine(points []calibration.AFPoint) (slope, offset float64) {
	var n, sx, sy, sxx, sxy float64
	for _, p := range points {
		if p.DistanceMM <= 0 {
			continue
		}
		x, y := 1000/float64(p.DistanceMM), float64(p.Code)
		n++
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	d := n*sxx - sx*sx
	if n < 2 || d == 0 {
		return 0, 0
	}
	slope = (n*sxy - sx*sy) / d
	offset = (sy - slope*sx) / n
	return slope, offset
}
