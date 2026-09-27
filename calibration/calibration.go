// Package calibration reads per-unit camera module calibration from the
// module's EEPROM (OTP) image: auto focus actuator codes, PDAF gain and DCC
// maps, white balance and, later, lens shading.
//
// EEPROM layouts are vendor-specific. Each layout is a Format; Parse tries
// the registered formats' Detect in order and uses the first that claims
// the image, so vendor handlers stay behind one interface and the generic
// CamX layout (driven by the sensor module's EEPROMDriverData memory map) is
// the fallback.
package calibration

import (
	"errors"
	"fmt"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// AFPoint is a factory focus calibration point: the actuator code that
// focuses at a chart distance.
type AFPoint struct {
	DistanceMM int
	Code       int
}

// AF is a module's auto focus calibration.
type AF struct {
	// InfinityCode and MacroCode bound the lens travel as calibrated.
	InfinityCode int
	MacroCode    int
	// Points, if any, are calibration points at known distances.
	Points []AFPoint
	// InfinityMargin and MacroMargin, from the sensor module, extend the
	// travel by that fraction of MacroCode - InfinityCode, as CamX does.
	InfinityMargin float64
	MacroMargin    float64
}

// PDAF is a module's phase detection calibration, in Qualcomm's layout
// (vendor EEPROM libraries hand CamX the same structure): a gain map per PD
// pixel type and a defocus conversion coefficient (DCC) map.
type PDAF struct {
	// Version is the calibration's version as stored.
	Version int
	// DCCMode, GainMode and Orientation are the calibration's mode bytes.
	DCCMode, GainMode, Orientation int
	// GainWidth x GainHeight grid points span the whole image, corner to
	// corner; row-major.
	GainWidth, GainHeight int
	// GainQ is the gains' fixed-point fraction bits. Layouts that do not
	// store it set what the format implies.
	GainQ int
	// GainLeft and GainRight are per-PD-pixel-type gains, scaled by
	// 1 << GainQ. They flatten each type's vignetting (about 1.0 at the
	// optical centre, 2-3 in the corners) and so also equalise the two
	// types. The names are
	// the EEPROM's: on the S5KJN1, GainLeft applies to the PD lines
	// V4L2_METADATA_LAYOUT_S5KJN1_PDAF calls R, GainRight to its L lines
	// (measured: only that pairing flattens L/R).
	GainLeft, GainRight []int
	// DCCWidth x DCCHeight regions tile the image; row-major.
	DCCWidth, DCCHeight int
	// DCCQ is the DCC values' fixed-point fraction bits.
	DCCQ int
	// DCC is the lens movement, in actuator codes, per pixel of phase
	// (pixels of the binned image, 4080 wide on the S5KJN1), scaled by
	// 1 << DCCQ.
	DCC []int
}

// AWB is a module's white balance calibration: the white point of a grey
// chart, as R/G, B/G and Gr/Gb, measured on this unit and on the golden
// module the tuning was made with, under the same light.
type AWB struct {
	UnitRG, UnitBG, UnitGrGb       float64
	GoldenRG, GoldenBG, GoldenGrGb float64
}

// Factors are what CamX scales the tuning's AWB reference points by for
// this unit: unit over golden, for R/G and B/G.
func (a *AWB) Factors() (rg, bg float64) {
	return a.UnitRG / a.GoldenRG, a.UnitBG / a.GoldenBG
}

// Calibration is what a Format extracts from an EEPROM image.
type Calibration struct {
	// Format names the Format that parsed it.
	Format string
	// ModuleID identifies the unit, when the layout records it.
	ModuleID string
	AF       *AF
	PDAF     *PDAF
	AWB      *AWB
}

// Module is what the sensor module file says about its EEPROM.
type Module struct {
	// EEPROMName names the EEPROM driver; vendors encode the layout in it.
	EEPROMName string
	// AFAvailable is set when the generic memory map below is valid.
	AFAvailable    bool
	BigEndian      bool
	Macro          Location
	Infinity       Location
	MacroMargin    float64
	InfinityMargin float64
}

// Location is a value's place in the EEPROM, as the memory map gives it.
type Location struct {
	Offset int
	Mask   uint32
}

// Format is one EEPROM layout.
type Format interface {
	// Name identifies the format.
	Name() string
	// Detect reports whether the image is in this format. It must be
	// conservative: a false positive yields wrong calibration.
	Detect(m *Module, image []byte) bool
	// Parse extracts the calibration.
	Parse(m *Module, image []byte) (*Calibration, error)
}

// formats are tried in order: vendor layouts first, the generic one last.
var formats []Format

// Register adds a format ahead of the generic fallback.
func Register(f Format) {
	formats = append(formats, f)
}

// ErrUnknownFormat is returned when no format claims an image.
var ErrUnknownFormat = errors.New("no known EEPROM format matches")

// Parse extracts calibration from an EEPROM image with the first format
// that detects it.
func Parse(m *Module, image []byte) (*Calibration, error) {
	for _, f := range append(append([]Format{}, formats...), generic{}) {
		if f.Detect(m, image) {
			c, err := f.Parse(m, image)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name(), err)
			}
			c.Format = f.Name()
			return c, nil
		}
	}
	return nil, ErrUnknownFormat
}

// ModuleFromFile reads the EEPROM description from a sensor module file.
func ModuleFromFile(f *chromatix.File) (*Module, error) {
	var sym *chromatix.Symbol
	for _, s := range f.Modules() {
		if s.Name == "EEPROMDriverData" {
			ss := s
			sym = &ss
			break
		}
	}
	if sym == nil {
		return nil, errors.New("no EEPROMDriverData")
	}
	v, err := f.DecodePrefix(*sym, isp.EEPROMDriverDataPrefix)
	if err != nil {
		return nil, err
	}
	o := v.(*chromatix.Object)
	loc := func(name string) Location {
		l := get(o, name).(*chromatix.Object)
		return Location{Offset: int(get(l, "offset").(uint64)), Mask: uint32(get(l, "mask").(uint64))}
	}
	return &Module{
		EEPROMName:     get(o, "EEPROMName").(string),
		AFAvailable:    get(o, "afAvailable").(uint64) != 0,
		BigEndian:      get(o, "afEndianness").(string) == "BIG",
		Macro:          loc("macro"),
		Infinity:       loc("infinity"),
		MacroMargin:    get(o, "macroMargin").(float64),
		InfinityMargin: get(o, "infinityMargin").(float64),
	}, nil
}

func get(o *chromatix.Object, name string) chromatix.Value {
	v, ok := o.Get(name)
	if !ok {
		panic("schema has no field " + name)
	}
	return v
}

// be16 reads a big-endian 16-bit value.
func be16(b []byte, off int) int {
	return int(b[off])<<8 | int(b[off+1])
}
