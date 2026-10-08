package calibration

import (
	"math"
	"testing"
)

// ovImage builds a synthetic OTP image with the blocks in the given bank
// (the other bank blank).
func ovImage(bank int) []byte {
	img := make([]byte, ovSize)
	b := img[bank*ovBankSize : (bank+1)*ovBankSize]

	b[ovInfoOff] = ovFlag
	copy(b[ovPartOff:], "28E65604")
	put16(b, ovInfoCRCOff, int(CRC16Buypass(b[ovInfoOff:ovInfoOff+ovInfoLen])))

	b[ovWBOff] = ovFlag
	for i := 0; i < 4; i++ {
		b[ovWBTolOff+i] = 50
	}
	put16(b, ovWBGoldenOff, 9608)
	put16(b, ovWBGoldenOff+2, 9894)
	put16(b, ovWBGoldenOff+4, 16388)
	put16(b, ovWBUnitOff, 9557)
	put16(b, ovWBUnitOff+2, 10012)
	put16(b, ovWBUnitOff+4, 16395)
	put16(b, ovWBCRCOff, int(CRC16Buypass(b[ovWBOff:ovWBOff+ovWBLen])))

	b[ovLSCOff] = ovFlag
	for ch := 0; ch < 4; ch++ {
		for i := 0; i < ovLSCPoints; i++ {
			put16(b, ovLSCTables+2*(ch*ovLSCPoints+i), 1000*(ch+1)+i)
		}
	}
	put16(b, ovLSCCRCOff, int(CRC16Buypass(b[ovLSCOff:ovLSCOff+ovLSCLen])))
	return img
}

func TestMotorolaOV08F(t *testing.T) {
	m := &Module{EEPROMName: "mot_ov08f_otp"}
	for bank := 0; bank < 2; bank++ {
		c, err := Parse(m, ovImage(bank))
		if err != nil {
			t.Fatalf("bank %d: %v", bank, err)
		}
		if c.Format != "motorola-ov08f-otp" || c.ModuleID != "28E65604" {
			t.Errorf("bank %d: format %q module %q", bank, c.Format, c.ModuleID)
		}
		if c.AF != nil {
			t.Errorf("bank %d: AF %v", bank, c.AF)
		}
		if c.AWB == nil || math.Abs(c.AWB.GoldenRG-9608.0/16384) > 1e-9 ||
			math.Abs(c.AWB.UnitBG-10012.0/16384) > 1e-9 {
			t.Errorf("bank %d: AWB %+v", bank, c.AWB)
		}
		if c.LSC == nil || len(c.LSC.R) != 221 || c.LSC.R[5] != 1005 ||
			c.LSC.Gr[0] != 2000 || c.LSC.Gb[220] != 3220 || c.LSC.B[1] != 4001 {
			t.Errorf("bank %d: LSC %+v", bank, c.LSC)
		}
	}
}

func TestMotorolaOV08FRejects(t *testing.T) {
	m := &Module{EEPROMName: "mot_ov08f_otp"}

	img := ovImage(0)
	img[ovPartOff] ^= 0xff
	if _, err := Parse(m, img); err == nil {
		t.Error("a corrupt module block was accepted")
	}
	if _, err := Parse(m, ovImage(0)[:ovSize-1]); err == nil {
		t.Error("a short image was accepted")
	}
	if _, err := Parse(&Module{EEPROMName: "other"}, ovImage(0)); err == nil {
		t.Error("another EEPROM's name was accepted")
	}

	// A unit outside the golden module's tolerances has no AWB
	img = ovImage(0)
	put16(img, ovWBUnitOff, 12000)
	put16(img, ovWBCRCOff, int(CRC16Buypass(img[ovWBOff:ovWBOff+ovWBLen])))
	c, err := Parse(&Module{EEPROMName: "mot_ov08f_otp"}, img)
	if err != nil {
		t.Fatal(err)
	}
	if c.AWB != nil {
		t.Error("an out of tolerance unit has AWB")
	}
}
