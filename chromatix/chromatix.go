// Package chromatix reads Qualcomm CamX Chromatix parameter files: the
// com.qti.tuned.*.bin and com.qti.sensormodule.*.bin blobs found under
// /vendor/lib64/camera on Qualcomm Android devices.
//
// The container is self-describing. A fixed header points to a section
// table; section 0 is a symbol table naming every parameter element, section
// 1 holds the element data, and section 2 is an index of the top-level
// modules. Inside an element's data, scalars are stored inline, while
// strings and repeated elements are stored as a (count, symbol id) pair
// referring to another symbol whose data holds them. What the inline fields
// mean is defined per module (see the module decoders); this package only
// exposes the container.
//
// All integers are little endian.
package chromatix

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// Magic is the string every Chromatix parameter file starts with.
const Magic = "QTI Chromatix Header"

const (
	offFileSize     = 0x1c
	offParser       = 0x28
	lenParser       = 0x70
	offSectionTable = 0x98
	sectionSize     = 24
	symbolSize      = 0x48
	symbolNameLen   = 32
)

// Section types.
const (
	SectionSymbols = 0
	SectionData    = 1
	SectionIndex   = 2
)

// NoGroup is the group field of symbols that are not top-level modules.
const NoGroup = 0xffffffff

// Section is one entry of the section table.
type Section struct {
	Type   uint64
	Offset uint64
	Size   uint64
}

// Symbol is one entry of the symbol table.
type Symbol struct {
	// ID is how data elsewhere refers to this symbol. IDs are unique but
	// need not be dense or start at 1.
	ID uint64
	// Name is the parameter element's name, truncated to 31 bytes by the
	// format (hence names such as "mod_swmc-_total_scale_ratio_data").
	Name string
	// Version is the module version for top-level modules, packed as
	// major | minor<<16 | incr<<32, and 0 otherwise.
	Version uint64
	// Group is the module group for top-level modules (software
	// algorithms, 3A, per-block hardware sets and per-usecase overrides),
	// and NoGroup otherwise.
	Group uint64
	// Offset and Size locate the symbol's data within the data section.
	Offset uint64
	Size   uint64
}

// IsModule reports whether the symbol is a top-level module.
func (s Symbol) IsModule() bool {
	return s.Group != NoGroup
}

// VersionString formats Version as major.minor.incr.
func (s Symbol) VersionString() string {
	return fmt.Sprintf("%d.%d.%d", s.Version&0xffff, (s.Version>>16)&0xffff, s.Version>>32)
}

// File is a parsed Chromatix parameter file.
type File struct {
	// Parser is the version string of the tool that wrote the file.
	Parser   string
	Sections []Section
	Symbols  []Symbol

	data []byte
	byID map[uint64]int
}

// Parse parses a Chromatix parameter file held in memory. The returned File
// keeps a reference to buf.
func Parse(buf []byte) (*File, error) {
	if len(buf) < offSectionTable+8 || !bytes.HasPrefix(buf, []byte(Magic)) {
		return nil, errors.New("not a Chromatix parameter file")
	}
	le := binary.LittleEndian

	if size := le.Uint32(buf[offFileSize:]); int(size) != len(buf) {
		return nil, fmt.Errorf("header gives file size %d, file is %d bytes", size, len(buf))
	}

	f := &File{
		Parser: cString(buf[offParser : offParser+lenParser]),
		byID:   map[uint64]int{},
	}

	tableOff := uint64(le.Uint32(buf[offSectionTable:]))
	count := uint64(le.Uint32(buf[offSectionTable+4:]))
	if tableOff+count*sectionSize > uint64(len(buf)) {
		return nil, errors.New("section table out of bounds")
	}
	var symbols, data *Section
	for i := uint64(0); i < count; i++ {
		b := buf[tableOff+i*sectionSize:]
		s := Section{le.Uint64(b), le.Uint64(b[8:]), le.Uint64(b[16:])}
		if s.Offset+s.Size > uint64(len(buf)) {
			return nil, fmt.Errorf("section %d out of bounds", s.Type)
		}
		f.Sections = append(f.Sections, s)
		switch s.Type {
		case SectionSymbols:
			symbols = &f.Sections[len(f.Sections)-1]
		case SectionData:
			data = &f.Sections[len(f.Sections)-1]
		}
	}
	if symbols == nil || data == nil {
		return nil, errors.New("missing symbol or data section")
	}
	if symbols.Size%symbolSize != 0 {
		return nil, fmt.Errorf("symbol table size %#x is not a multiple of %#x",
			symbols.Size, symbolSize)
	}

	f.data = buf[data.Offset : data.Offset+data.Size]

	for i := uint64(0); i < symbols.Size/symbolSize; i++ {
		b := buf[symbols.Offset+i*symbolSize:]
		s := Symbol{
			ID:      le.Uint64(b),
			Name:    cString(b[8 : 8+symbolNameLen]),
			Version: le.Uint64(b[40:]),
			Group:   le.Uint64(b[48:]),
			Offset:  le.Uint64(b[56:]),
			Size:    le.Uint64(b[64:]),
		}
		if s.Offset+s.Size > uint64(len(f.data)) {
			return nil, fmt.Errorf("symbol %#x (%s) data out of bounds", s.ID, s.Name)
		}
		if _, dup := f.byID[s.ID]; dup {
			return nil, fmt.Errorf("duplicate symbol id %#x", s.ID)
		}
		f.byID[s.ID] = len(f.Symbols)
		f.Symbols = append(f.Symbols, s)
	}

	return f, nil
}

// Symbol returns the symbol with the given ID.
func (f *File) Symbol(id uint64) (Symbol, bool) {
	i, ok := f.byID[id]
	if !ok {
		return Symbol{}, false
	}
	return f.Symbols[i], true
}

// Modules returns the top-level modules, in file order.
func (f *File) Modules() []Symbol {
	var m []Symbol
	for _, s := range f.Symbols {
		if s.IsModule() {
			m = append(m, s)
		}
	}
	return m
}

// Data returns the data of a symbol.
func (f *File) Data(s Symbol) []byte {
	return f.data[s.Offset : s.Offset+s.Size]
}

func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
