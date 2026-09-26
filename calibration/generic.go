package calibration

import (
	"errors"
	"math/bits"
)

// generic is the CamX layout: the sensor module's EEPROMDriverData memory
// map gives each value's offset and mask directly. It is the fallback, and
// only applies when the module marks its auto focus data available.
type generic struct{}

func (generic) Name() string { return "camx-generic" }

func (generic) Detect(m *Module, image []byte) bool {
	return m != nil && m.AFAvailable &&
		m.Macro.Offset+2 <= len(image) && m.Infinity.Offset+2 <= len(image)
}

func (generic) Parse(m *Module, image []byte) (*Calibration, error) {
	read := func(l Location) int {
		var v uint32
		if bits.Len32(l.Mask) <= 8 {
			v = uint32(image[l.Offset])
		} else if m.BigEndian {
			v = uint32(be16(image, l.Offset))
		} else {
			v = uint32(image[l.Offset]) | uint32(image[l.Offset+1])<<8
		}
		if l.Mask != 0 {
			v &= l.Mask
		}
		return int(v)
	}
	af := &AF{
		InfinityCode:   read(m.Infinity),
		MacroCode:      read(m.Macro),
		InfinityMargin: m.InfinityMargin,
		MacroMargin:    m.MacroMargin,
	}
	if af.MacroCode == af.InfinityCode {
		return nil, errors.New("macro and infinity codes are equal")
	}
	return &Calibration{AF: af}, nil
}
