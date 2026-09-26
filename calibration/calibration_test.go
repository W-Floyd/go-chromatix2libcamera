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

	// PDAF: a 17x13 gain map (left i, right 1000+i) and an 8x6 DCC map
	// (Q4, 250+i).
	copy(b[motGainOff:], "110")
	put16(b, motGainOff+3, 9)
	put16(b, motGainOff+5, 17)
	put16(b, motGainOff+7, 13)
	for i := 0; i < 17*13; i++ {
		put16(b, motGainOff+9+2*i, i)
		put16(b, motGainOff+9+2*(17*13+i), 1000+i)
	}
	putCRC(b, motGainOff, motGainLen, motGainCRCOff)
	put16(b, motDCCOff, 4)
	put16(b, motDCCOff+2, 8)
	put16(b, motDCCOff+4, 6)
	for i := 0; i < 8*6; i++ {
		put16(b, motDCCOff+6+2*i, 250+i)
	}
	putCRC(b, motDCCOff, motDCCLen, motDCCCRCOff)
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

func TestMotorolaPDAF(t *testing.T) {
	c, err := Parse(motModule, motImage())
	if err != nil {
		t.Fatal(err)
	}
	p := c.PDAF
	if p == nil {
		t.Fatal("no PDAF")
	}
	if p.Version != 9 || p.GainQ != motGainQ || p.DCCMode != '1' || p.GainMode != '1' || p.Orientation != '0' {
		t.Errorf("header %+v", p)
	}
	if p.GainWidth != 17 || p.GainHeight != 13 || len(p.GainLeft) != 221 || len(p.GainRight) != 221 {
		t.Fatalf("gain map %dx%d, %d/%d entries", p.GainWidth, p.GainHeight, len(p.GainLeft), len(p.GainRight))
	}
	if p.GainLeft[220] != 220 || p.GainRight[0] != 1000 || p.GainRight[220] != 1220 {
		t.Errorf("gains L[220] %d R[0] %d R[220] %d", p.GainLeft[220], p.GainRight[0], p.GainRight[220])
	}
	if p.DCCQ != 4 || p.DCCWidth != 8 || p.DCCHeight != 6 || len(p.DCC) != 48 || p.DCC[47] != 297 {
		t.Errorf("dcc q %d %dx%d %v", p.DCCQ, p.DCCWidth, p.DCCHeight, p.DCC)
	}
}

func TestMotorolaBadPDAFDropsPDAFOnly(t *testing.T) {
	for _, off := range []int{motGainCRCOff, motDCCCRCOff} {
		b := motImage()
		b[off] ^= 0xff
		c, err := Parse(motModule, b)
		if err != nil {
			t.Fatal(err)
		}
		if c.PDAF != nil || c.AF.MacroCode != 753 {
			t.Errorf("CRC at %#x broken: pdaf %+v af %+v", off, c.PDAF, c.AF)
		}
	}

	// A map that claims more entries than its block holds.
	b := motImage()
	put16(b, motGainOff+5, 25)
	put16(b, motGainOff+7, 19)
	putCRC(b, motGainOff, motGainLen, motGainCRCOff)
	if c, err := Parse(motModule, b); err != nil || c.PDAF != nil {
		t.Errorf("oversized gain map: pdaf %+v err %v", c.PDAF, err)
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
