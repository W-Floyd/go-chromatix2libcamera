package calibration

import (
	"fmt"
	"strings"
)

// motorolaGT24P256C is Motorola's layout for GT24P256C EEPROMs, as parsed
// by their com.mot.eeprom.<EEPROMName>.so library for CamX. Motorola ships
// one library per layout and names it after the EEPROM driver, so detection
// matches the EEPROM name exactly, then checks the image's size and the
// module information block's CRC.
//
// Layout (offsets in bytes, all values big-endian; codes are 10-bit
// actuator codes in the top bits of 16):
//
//	0x00-0x0b  module serial (ASCII)
//	0x27-0x3e  module information block, CRC-16/BUYPASS at 0x3f
//	0x29       macro code << 6
//	0x2d       infinity code << 6
//	0x18f6     four AF calibration points: distance (mm), code << 6
//	0x1906     their CRC-16/BUYPASS
type motorolaGT24P256C struct{}

// motorolaGT24P256CNames are the EEPROM drivers known to use this layout.
var motorolaGT24P256CNames = []string{
	"mot_gt24p256c_s5kjns_eeprom",
}

const (
	motSize        = 6777
	motInfoOff     = 0x27
	motInfoLen     = 24
	motInfoCRCOff  = 0x3f
	motMacroOff    = 0x29
	motInfinityOff = 0x2d
	motAFOff       = 0x18f6
	motAFPoints    = 4
	motAFCRCOff    = motAFOff + 4*motAFPoints
	motCodeShift   = 6
)

func init() { Register(motorolaGT24P256C{}) }

func (motorolaGT24P256C) Name() string { return "motorola-gt24p256c" }

func (motorolaGT24P256C) Detect(m *Module, image []byte) bool {
	if m == nil || len(image) < motSize {
		return false
	}
	known := false
	for _, n := range motorolaGT24P256CNames {
		known = known || m.EEPROMName == n
	}
	return known && crcOK(image, motInfoOff, motInfoLen, motInfoCRCOff)
}

func (motorolaGT24P256C) Parse(m *Module, image []byte) (*Calibration, error) {
	af := &AF{
		MacroCode:      be16(image, motMacroOff) >> motCodeShift,
		InfinityCode:   be16(image, motInfinityOff) >> motCodeShift,
		InfinityMargin: m.InfinityMargin,
		MacroMargin:    m.MacroMargin,
	}
	if af.MacroCode <= af.InfinityCode {
		return nil, fmt.Errorf("macro code %d not above infinity code %d",
			af.MacroCode, af.InfinityCode)
	}

	if crcOK(image, motAFOff, 4*motAFPoints, motAFCRCOff) {
		for i := 0; i < motAFPoints; i++ {
			o := motAFOff + 4*i
			af.Points = append(af.Points, AFPoint{
				DistanceMM: be16(image, o),
				Code:       be16(image, o+2) >> motCodeShift,
			})
		}
	}

	return &Calibration{
		ModuleID: strings.TrimRight(string(image[:12]), "\x00\xff "),
		AF:       af,
	}, nil
}

func crcOK(image []byte, off, n, crcOff int) bool {
	if crcOff+2 > len(image) {
		return false
	}
	return CRC16Buypass(image[off:off+n]) == uint16(be16(image, crcOff))
}
