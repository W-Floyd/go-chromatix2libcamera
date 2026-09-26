package calibration

import (
	"errors"
	"testing"
)

func put16(b []byte, off, v int) {
	b[off] = byte(v >> 8)
	b[off+1] = byte(v)
}

func putCRC(b []byte, off, n, crcOff int) {
	put16(b, crcOff, int(CRC16Buypass(b[off:off+n])))
}

// motImage builds a synthetic image in Motorola's GT24P256C layout.
func motImage() []byte {
	b := make([]byte, motSize)
	for i := range b {
		b[i] = 0xff
	}
	copy(b, "SERIAL000001")
	put16(b, motMacroOff, 753<<motCodeShift)
	put16(b, motInfinityOff, 525<<motCodeShift)
	putCRC(b, motInfoOff, motInfoLen, motInfoCRCOff)
	pts := [][2]int{{300, 597}, {500, 567}, {750, 552}, {10000, 525}}
	for i, p := range pts {
		put16(b, motAFOff+4*i, p[0])
		put16(b, motAFOff+4*i+2, p[1]<<motCodeShift)
	}
	putCRC(b, motAFOff, 4*motAFPoints, motAFCRCOff)
	return b
}

var motModule = &Module{
	EEPROMName:     "mot_gt24p256c_s5kjns_eeprom",
	MacroMargin:    0.2,
	InfinityMargin: -0.14,
}

func TestCRC16Buypass(t *testing.T) {
	// The standard check value for CRC-16/BUYPASS.
	if got := CRC16Buypass([]byte("123456789")); got != 0xfee8 {
		t.Errorf("CRC16Buypass(123456789) = %#04x, want 0xfee8", got)
	}
}

func TestMotorola(t *testing.T) {
	c, err := Parse(motModule, motImage())
	if err != nil {
		t.Fatal(err)
	}
	if c.Format != "motorola-gt24p256c" || c.ModuleID != "SERIAL000001" {
		t.Errorf("format %q module %q", c.Format, c.ModuleID)
	}
	af := c.AF
	if af.InfinityCode != 525 || af.MacroCode != 753 || len(af.Points) != 4 {
		t.Fatalf("af = %+v", af)
	}
	if af.Points[0] != (AFPoint{300, 597}) || af.MacroMargin != 0.2 {
		t.Errorf("af = %+v", af)
	}
}

func TestMotorolaBadAFCRCDropsPointsOnly(t *testing.T) {
	b := motImage()
	b[motAFCRCOff] ^= 0xff
	c, err := Parse(motModule, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.AF.Points) != 0 || c.AF.MacroCode != 753 {
		t.Errorf("af = %+v", c.AF)
	}
}

func TestMotorolaNeedsNameAndInfoCRC(t *testing.T) {
	other := *motModule
	other.EEPROMName = "someone_else_eeprom"
	if _, err := Parse(&other, motImage()); !errors.Is(err, ErrUnknownFormat) {
		t.Errorf("unknown EEPROM name: err = %v", err)
	}

	b := motImage()
	b[motInfoOff] ^= 0xff
	if _, err := Parse(motModule, b); !errors.Is(err, ErrUnknownFormat) {
		t.Errorf("bad info CRC: err = %v", err)
	}
}

func TestGenericFallback(t *testing.T) {
	b := make([]byte, 64)
	put16(b, 0x10, 0x0300)
	put16(b, 0x12, 0x0100)
	m := &Module{
		EEPROMName:  "generic_eeprom",
		AFAvailable: true,
		BigEndian:   true,
		Macro:       Location{0x10, 0x3ff},
		Infinity:    Location{0x12, 0x3ff},
	}
	c, err := Parse(m, b)
	if err != nil {
		t.Fatal(err)
	}
	if c.Format != "camx-generic" || c.AF.MacroCode != 0x300 || c.AF.InfinityCode != 0x100 {
		t.Errorf("c = %+v af = %+v", c, c.AF)
	}
}
