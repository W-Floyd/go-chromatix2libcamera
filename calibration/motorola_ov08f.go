package calibration

import (
	"fmt"
	"strings"
)

// motorolaOV08F is Motorola's layout for the OTP of the front OV08F sensor,
// as parsed by their com.mot.eeprom.mot_ov08f_otp.so library for CamX, and
// checked against a unit's OTP (every CRC passes). The OTP is 60 pages of
// 128 bytes (sensor pages 0x04-0x3f) in two banks of 0xf00 bytes; each
// block comes from the first bank where its flag byte is 0x40 and its CRC
// passes. Values are big-endian.
//
//	0x00-0x25  module information block, CRC-16/BUYPASS at 0x26: flag,
//	           revision, part number (8 ASCII characters at 0x04), lens
//	           id at 0x0d, integrator (two letters) at 0x0e, factory at
//	           0x10, line at 0x12, date (year, month, day) at 0x13,
//	           serial number at 0x16
//	0x28-0x53  white balance block, CRC at 0x54: at 0x2f the R/G and B/G
//	           tolerances (min, max; one byte each, in 1/1000), at 0x3b
//	           the golden module's R/G, B/G, Gr/Gb, at 0x49 the unit's
//	           (16-bit each, in 1/16384), under D50
//	0x9c-0x784 lens shading block, CRC at 0x785: a flat field's level at
//	           each of 17x13 points, for R, Gr, Gb, B (16-bit each,
//	           1023 at the centre), under D50, in the sensor's own
//	           orientation (the library does not turn them)
//
// The unit has no AF data (a fixed focus lens). The tolerance bytes'
// order and the lens shading channel order are inferred from the
// S5KJNS layout and the data's look (R falls off most, B least; Gr and
// Gb nearly equal).
type motorolaOV08F struct{}

var motorolaOV08FNames = []string{
	"mot_ov08f_otp",
}

const (
	ovSize     = 0x1e00
	ovBankSize = 0xf00
	ovFlag     = 0x40

	ovInfoOff    = 0x00
	ovInfoLen    = 0x26
	ovInfoCRCOff = 0x26
	ovPartOff    = 0x04
	ovPartLen    = 8

	ovWBOff       = 0x28
	ovWBLen       = 0x2c
	ovWBCRCOff    = ovWBOff + ovWBLen
	ovWBTolOff    = 0x2f
	ovWBGoldenOff = 0x3b
	ovWBUnitOff   = 0x49

	ovLSCOff    = 0x9c
	ovLSCTables = 0x9d
	ovLSCLen    = 0x784 - 0x9c + 1
	ovLSCCRCOff = ovLSCOff + ovLSCLen
	ovLSCPoints = 17 * 13
)

func init() { Register(motorolaOV08F{}) }

func (motorolaOV08F) Name() string { return "motorola-ov08f-otp" }

// ovBlock returns the first bank's copy of a block that has its flag and
// passes its CRC, as a slice of the image starting at the bank, or nil.
func ovBlock(image []byte, off, n, crcOff int) []byte {
	for bank := 0; bank < 2; bank++ {
		b := image[bank*ovBankSize : (bank+1)*ovBankSize]
		if b[off] == ovFlag && crcOK(b, off, n, crcOff) {
			return b
		}
	}
	return nil
}

func (motorolaOV08F) Detect(m *Module, image []byte) bool {
	if m == nil || len(image) != ovSize {
		return false
	}
	known := false
	for _, n := range motorolaOV08FNames {
		known = known || m.EEPROMName == n
	}
	return known && ovBlock(image, ovInfoOff, ovInfoLen, ovInfoCRCOff) != nil
}

func (motorolaOV08F) Parse(m *Module, image []byte) (*Calibration, error) {
	info := ovBlock(image, ovInfoOff, ovInfoLen, ovInfoCRCOff)
	c := &Calibration{
		ModuleID: strings.TrimRight(string(info[ovPartOff:ovPartOff+ovPartLen]), "\x00\xff "),
	}

	if b := ovBlock(image, ovWBOff, ovWBLen, ovWBCRCOff); b != nil {
		c.AWB = ovAWB(b)
	}
	if b := ovBlock(image, ovLSCOff, ovLSCLen, ovLSCCRCOff); b != nil {
		c.LSC = ovLSC(b)
	}
	if c.AWB == nil && c.LSC == nil {
		return nil, fmt.Errorf("no valid white balance or lens shading block")
	}
	return c, nil
}

// ovAWB reads the white balance block; nil if, as the library rejects it,
// the unit is outside the golden module's tolerances.
func ovAWB(b []byte) *AWB {
	ratio := func(off int) float64 { return float64(be16(b, off)) / motWBScale }
	tol := func(i int) float64 { return float64(b[ovWBTolOff+i]) / motWBTolScale }
	a := &AWB{
		GoldenRG: ratio(ovWBGoldenOff), GoldenBG: ratio(ovWBGoldenOff + 2),
		GoldenGrGb: ratio(ovWBGoldenOff + 4),
		UnitRG:     ratio(ovWBUnitOff), UnitBG: ratio(ovWBUnitOff + 2),
		UnitGrGb: ratio(ovWBUnitOff + 4),
	}
	if a.GoldenRG == 0 || a.GoldenBG == 0 ||
		a.UnitRG < a.GoldenRG-tol(0) || a.UnitRG > a.GoldenRG+tol(1) ||
		a.UnitBG < a.GoldenBG-tol(2) || a.UnitBG > a.GoldenBG+tol(3) {
		return nil
	}
	return a
}

// ovLSC reads the lens shading tables, in the order stored.
func ovLSC(b []byte) *LSC {
	table := func(ch int) []float64 {
		t := make([]float64, ovLSCPoints)
		for i := range t {
			t[i] = float64(be16(b, ovLSCTables+2*(ch*ovLSCPoints+i)))
		}
		return t
	}
	return &LSC{R: table(0), Gr: table(1), Gb: table(2), B: table(3)}
}
