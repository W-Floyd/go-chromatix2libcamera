package chromatix

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"
)

// builder assembles a synthetic Chromatix file, so that the tests need no
// vendor data.
type builder struct {
	symbols []Symbol
	data    []byte
}

func (b *builder) add(name string, version, group uint64, data []byte) uint64 {
	id := uint64(len(b.symbols) + 1)
	b.symbols = append(b.symbols, Symbol{ID: id, Name: name, Version: version,
		Group: group, Offset: uint64(len(b.data)), Size: uint64(len(data))})
	b.data = append(b.data, data...)
	return id
}

func slots(words ...uint64) []byte {
	out := make([]byte, 8*len(words))
	for i, w := range words {
		binary.LittleEndian.PutUint64(out[8*i:], w)
	}
	return out
}

func fbits(f float32) uint64 { return uint64(math.Float32bits(f)) }

func (b *builder) bytes() []byte {
	le := binary.LittleEndian
	const hdr = 0xe8
	symOff := uint64(hdr)
	symSize := uint64(len(b.symbols) * symbolSize)
	dataOff := symOff + symSize
	total := dataOff + uint64(len(b.data))

	buf := make([]byte, total)
	copy(buf, Magic)
	le.PutUint32(buf[offFileSize:], uint32(total))
	copy(buf[offParser:], "test")
	le.PutUint32(buf[offSectionTable:], 0xa0)
	le.PutUint32(buf[offSectionTable+4:], 2)
	for i, s := range []Section{{SectionSymbols, symOff, symSize}, {SectionData, dataOff, uint64(len(b.data))}} {
		o := 0xa0 + i*sectionSize
		le.PutUint64(buf[o:], s.Type)
		le.PutUint64(buf[o+8:], s.Offset)
		le.PutUint64(buf[o+16:], s.Size)
	}
	for i, s := range b.symbols {
		o := int(symOff) + i*symbolSize
		le.PutUint64(buf[o:], s.ID)
		copy(buf[o+8:o+40], s.Name)
		le.PutUint64(buf[o+40:], s.Version)
		le.PutUint64(buf[o+48:], s.Group)
		le.PutUint64(buf[o+56:], s.Offset)
		le.PutUint64(buf[o+64:], s.Size)
	}
	copy(buf[dataOff:], b.data)
	return buf
}

func TestParseAndDecode(t *testing.T) {
	var b builder
	str := b.add("revision", 0, NoGroup, []byte("0\x00\x00\x00\x00\x00\x00\x00"))
	// Two list elements of {trigger start, end; packed 3 floats}.
	packed := func(a, c, d float32) []byte {
		p := make([]byte, 16)
		binary.LittleEndian.PutUint32(p, math.Float32bits(a))
		binary.LittleEndian.PutUint32(p[4:], math.Float32bits(c))
		binary.LittleEndian.PutUint32(p[8:], math.Float32bits(d))
		return p
	}
	var elems []byte
	elems = append(elems, slots(fbits(1), fbits(2))...)
	elems = append(elems, packed(0.5, 1.5, 2.5)...)
	elems = append(elems, slots(fbits(3), fbits(4))...)
	elems = append(elems, packed(3.5, 4.5, 5.5)...)
	list := b.add("elements", 0, NoGroup, elems)
	b.add("test12_ope", 1|2<<16, 0xa, slots(1, 2, str, 1, 2, list))

	f, err := Parse(b.bytes())
	if err != nil {
		t.Fatal(err)
	}
	mods := f.Modules()
	if len(mods) != 1 || mods[0].Name != "test12_ope" || mods[0].VersionString() != "1.2.0" {
		t.Fatalf("modules: %+v", mods)
	}

	schema := TStruct(
		F("enable", TInt),
		F("revision", TString),
		F("enum", TEnum("a", "b")),
		F("regions", TList(TStruct(
			F("trigger", TStruct(F("start", TFloat), F("end", TFloat))),
			F("values", TPacked(3, TFloat)),
		))),
	)
	v, err := f.Decode(mods[0], schema)
	if err != nil {
		t.Fatal(err)
	}
	o := v.(*Object)
	if got, _ := o.Get("revision"); got != "0" {
		t.Errorf("revision = %v", got)
	}
	if got, _ := o.Get("enum"); got != "b" {
		t.Errorf("enum = %v", got)
	}
	regions, _ := o.Get("regions")
	r1 := regions.([]Value)[1].(*Object)
	vals, _ := r1.Get("values")
	if want := []Value{3.5, 4.5, 5.5}; !reflect.DeepEqual(vals, want) {
		t.Errorf("values = %v, want %v", vals, want)
	}
}

func TestDecodeRejectsWrongSchemaSize(t *testing.T) {
	var b builder
	b.add("m_ope", 1, 0xa, slots(1, 2))
	f, err := Parse(b.bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Decode(f.Modules()[0], TStruct(F("x", TInt))); err == nil {
		t.Error("a schema covering 1 of 2 slots decoded without error")
	}
}
