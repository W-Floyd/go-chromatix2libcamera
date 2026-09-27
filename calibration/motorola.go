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
//	0x41-0x6b  white balance block, CRC at 0x6c: at 0x47 the R/G and
//	           B/G tolerances (min, max; one byte each, in 1/1000), at
//	           0x53 the golden module's R/G, B/G, Gr/Gb, at 0x61 the
//	           unit's (16-bit each, in 1/16384), under D50: the library
//	           hands CamX unit over golden as the WBCalibrationData of
//	           EEPROMIlluminantType D50
//	0xf7       lens shading block, CRC at 0x7df: a flat field's level at
//	           each of 17x13 points, for R, Gr, Gb, B (16-bit each), under
//	           D50; stored rotated 180 degrees ("MIRROR ON, FLIP ON"),
//	           so the library reverses each table
//	0x18f6     four AF calibration points: distance (mm), code << 6
//	0x1906     their CRC-16/BUYPASS
//	0x7e1      PDAF gain map block, 893 bytes, CRC at 0xb5e: DCC mode,
//	           gain mode, orientation (one byte each), version, width,
//	           height, then width*height left gains and as many right
//	           gains (16-bit each)
//	0xb60      PDAF DCC block, 102 bytes, CRC at 0xbc6: Q, width, height,
//	           then width*height DCC values
//
// The library rejects gain maps wider than 25 or taller than 19 and DCC
// maps over 200 entries; so does Parse. The gain map stores no Q, unlike
// the DCC; motGainQ is inferred from calibrated units, whose gain at the
// optical centre (where vignetting is nil) reads 256.
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

	motGainOff    = 0x7e1
	motGainLen    = 893
	motGainCRCOff = motGainOff + motGainLen
	motGainMaxW   = 25
	motGainMaxH   = 19
	motGainQ      = 8
	motDCCOff     = 0xb60
	motDCCLen     = 102
	motDCCCRCOff  = motDCCOff + motDCCLen
	motDCCMax     = 200

	motWBOff       = 0x41
	motWBLen       = 43
	motWBCRCOff    = motWBOff + motWBLen
	motWBTolOff    = 0x47
	motWBGoldenOff = 0x53
	motWBUnitOff   = 0x61
	motWBScale     = 1 << 14
	motWBTolScale  = 1000

	motLSCOff    = 0xf7
	motLSCPoints = 17 * 13
	motLSCLen    = 4 * 2 * motLSCPoints
	motLSCCRCOff = motLSCOff + motLSCLen
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
		PDAF:     motPDAF(image),
		AWB:      motAWB(image),
		LSC:      motLSC(image),
	}, nil
}

// motLSC reads the lens shading block, turned the library's way round; nil
// if it fails its CRC.
func motLSC(image []byte) *LSC {
	if !crcOK(image, motLSCOff, motLSCLen, motLSCCRCOff) {
		return nil
	}
	table := func(ch int) []float64 {
		t := make([]float64, motLSCPoints)
		for i := range t {
			t[motLSCPoints-1-i] = float64(be16(image, motLSCOff+2*(ch*motLSCPoints+i)))
		}
		return t
	}
	return &LSC{R: table(0), Gr: table(1), Gb: table(2), B: table(3)}
}

// motAWB reads the white balance block; nil if it fails its CRC or, as the
// library rejects it, the unit is outside the golden module's tolerances.
func motAWB(image []byte) *AWB {
	if !crcOK(image, motWBOff, motWBLen, motWBCRCOff) {
		return nil
	}
	ratio := func(off int) float64 { return float64(be16(image, off)) / motWBScale }
	tol := func(i int) float64 { return float64(image[motWBTolOff+i]) / motWBTolScale }
	a := &AWB{
		GoldenRG: ratio(motWBGoldenOff), GoldenBG: ratio(motWBGoldenOff + 2),
		GoldenGrGb: ratio(motWBGoldenOff + 4),
		UnitRG:     ratio(motWBUnitOff), UnitBG: ratio(motWBUnitOff + 2),
		UnitGrGb: ratio(motWBUnitOff + 4),
	}
	if a.GoldenRG == 0 || a.GoldenBG == 0 ||
		a.UnitRG < a.GoldenRG-tol(0) || a.UnitRG > a.GoldenRG+tol(1) ||
		a.UnitBG < a.GoldenBG-tol(2) || a.UnitBG > a.GoldenBG+tol(3) {
		return nil
	}
	return a
}

// motPDAF reads the PDAF blocks; nil if either fails its CRC or holds
// sizes that do not fit its block.
func motPDAF(image []byte) *PDAF {
	if !crcOK(image, motGainOff, motGainLen, motGainCRCOff) ||
		!crcOK(image, motDCCOff, motDCCLen, motDCCCRCOff) {
		return nil
	}
	p := &PDAF{
		DCCMode:     int(image[motGainOff]),
		GainMode:    int(image[motGainOff+1]),
		Orientation: int(image[motGainOff+2]),
		Version:     be16(image, motGainOff+3),
		GainWidth:   be16(image, motGainOff+5),
		GainHeight:  be16(image, motGainOff+7),
		GainQ:       motGainQ,
		DCCQ:        be16(image, motDCCOff),
		DCCWidth:    be16(image, motDCCOff+2),
		DCCHeight:   be16(image, motDCCOff+4),
	}
	n := p.GainWidth * p.GainHeight
	d := p.DCCWidth * p.DCCHeight
	if p.GainWidth > motGainMaxW || p.GainHeight > motGainMaxH || n == 0 ||
		9+4*n > motGainLen || d == 0 || d > motDCCMax || 6+2*d > motDCCLen {
		return nil
	}
	gains := motGainOff + 9
	for i := 0; i < n; i++ {
		p.GainLeft = append(p.GainLeft, be16(image, gains+2*i))
		p.GainRight = append(p.GainRight, be16(image, gains+2*(n+i)))
	}
	for i := 0; i < d; i++ {
		p.DCC = append(p.DCC, be16(image, motDCCOff+6+2*i))
	}
	return p
}

func crcOK(image []byte, off, n, crcOff int) bool {
	if crcOff+2 > len(image) {
		return false
	}
	return CRC16Buypass(image[off:off+n]) == uint16(be16(image, crcOff))
}
