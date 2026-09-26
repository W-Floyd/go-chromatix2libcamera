package chromatix

import (
	"encoding/binary"
	"fmt"
	"math"
)

// The data encoding, as observed in files written by Parameter Parser
// V2.0.0: every leaf value occupies one 8-byte slot (32-bit values in the
// low half); nested structures and fixed-length sequences are laid out
// inline, in schema order; strings and variable-length sequences are a
// (count, symbol id) pair of slots, the referenced symbol's data holding
// count elements back to back. The exception is a fixed-length array of
// numbers (an XSD list type such as a gamma table), which is packed at 4
// bytes per value and padded to a whole slot.
const slot = 8

// Kind is the kind of a schema type.
type Kind int

const (
	Int    Kind = iota // signed integer
	Uint               // unsigned integer
	Float              // 32-bit float
	Enum               // integer index into Type.Values
	Struct             // Type.Fields, inline
	Array              // Type.Len elements of Type.Elem, inline
	List               // (count, id) reference to elements of Type.Elem
	String             // (length, id) reference to NUL-terminated bytes
	Packed             // Type.Len 32-bit Int, Uint or Float values, packed
)

// Type describes how to decode a value.
type Type struct {
	Kind   Kind
	Fields []Field  // Struct
	Elem   *Type    // Array, List
	Len    int      // Array
	Values []string // Enum
}

// Field is a named member of a Struct.
type Field struct {
	Name string
	Type *Type
}

// Convenience constructors for writing module schemas.
var (
	TInt    = &Type{Kind: Int}
	TUint   = &Type{Kind: Uint}
	TFloat  = &Type{Kind: Float}
	TString = &Type{Kind: String}
)

func TStruct(fields ...Field) *Type   { return &Type{Kind: Struct, Fields: fields} }
func TArray(n int, elem *Type) *Type  { return &Type{Kind: Array, Len: n, Elem: elem} }
func TList(elem *Type) *Type          { return &Type{Kind: List, Elem: elem} }
func TEnum(values ...string) *Type    { return &Type{Kind: Enum, Values: values} }
func TPacked(n int, elem *Type) *Type { return &Type{Kind: Packed, Len: n, Elem: elem} }
func F(name string, t *Type) Field    { return Field{Name: name, Type: t} }

// Slots returns the number of slots a value of type t occupies inline.
func (t *Type) Slots() int {
	switch t.Kind {
	case Struct:
		n := 0
		for _, f := range t.Fields {
			n += f.Type.Slots()
		}
		return n
	case Array:
		return t.Len * t.Elem.Slots()
	case List, String:
		return 2
	case Packed:
		return (t.Len*4 + slot - 1) / slot
	default:
		return 1
	}
}

// Value is a decoded value: int64, uint64, float64, string (for String and
// Enum), []Value (Array, List) or *Object (Struct).
type Value any

// Object is a decoded Struct, keeping schema field order.
type Object struct {
	Names  []string
	Values []Value
}

// Get returns a field's value.
func (o *Object) Get(name string) (Value, bool) {
	for i, n := range o.Names {
		if n == name {
			return o.Values[i], true
		}
	}
	return nil, false
}

// Decode decodes a symbol's data with the given schema. The schema must
// account for the whole of the symbol's data.
func (f *File) Decode(s Symbol, t *Type) (Value, error) {
	d := f.Data(s)
	if want := t.Slots() * slot; want != len(d) {
		return nil, fmt.Errorf("%s: schema covers %d bytes, symbol has %d", s.Name, want, len(d))
	}
	v, _, err := f.decode(d, t, s.Name)
	return v, err
}

func (f *File) decode(d []byte, t *Type, path string) (Value, []byte, error) {
	le := binary.LittleEndian
	need := t.Slots() * slot
	if len(d) < need {
		return nil, nil, fmt.Errorf("%s: short data", path)
	}

	switch t.Kind {
	case Int:
		return int64(int32(le.Uint32(d))), d[slot:], nil
	case Uint:
		return uint64(le.Uint32(d)), d[slot:], nil
	case Float:
		return float64(math.Float32frombits(le.Uint32(d))), d[slot:], nil
	case Enum:
		i := le.Uint32(d)
		if int(i) >= len(t.Values) {
			return nil, nil, fmt.Errorf("%s: enum value %d out of range", path, i)
		}
		return t.Values[i], d[slot:], nil

	case Struct:
		o := &Object{}
		for _, fl := range t.Fields {
			var v Value
			var err error
			v, d, err = f.decode(d, fl.Type, path+"."+fl.Name)
			if err != nil {
				return nil, nil, err
			}
			o.Names = append(o.Names, fl.Name)
			o.Values = append(o.Values, v)
		}
		return o, d, nil

	case Array:
		vs := make([]Value, t.Len)
		for i := range vs {
			var err error
			vs[i], d, err = f.decode(d, t.Elem, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, nil, err
			}
		}
		return vs, d, nil

	case Packed:
		vs := make([]Value, t.Len)
		for i := range vs {
			w := le.Uint32(d[4*i:])
			switch t.Elem.Kind {
			case Int:
				vs[i] = int64(int32(w))
			case Uint:
				vs[i] = uint64(w)
			case Float:
				vs[i] = float64(math.Float32frombits(w))
			default:
				return nil, nil, fmt.Errorf("%s: packed arrays hold numbers only", path)
			}
		}
		return vs, d[need:], nil

	case List, String:
		count, id := le.Uint64(d), le.Uint64(d[slot:])
		rest := d[2*slot:]
		if count == 0 {
			if t.Kind == String {
				return "", rest, nil
			}
			return []Value{}, rest, nil
		}
		ref, ok := f.Symbol(id)
		if !ok {
			return nil, nil, fmt.Errorf("%s: reference to missing symbol %#x", path, id)
		}
		rd := f.Data(ref)
		if t.Kind == String {
			if uint64(len(rd)) < count {
				return nil, nil, fmt.Errorf("%s: string longer than its symbol", path)
			}
			return cString(rd[:count]), rest, nil
		}
		if want := count * uint64(t.Elem.Slots()*slot); want != uint64(len(rd)) {
			return nil, nil, fmt.Errorf("%s: %d elements need %d bytes, symbol %s has %d",
				path, count, want, ref.Name, len(rd))
		}
		vs := make([]Value, count)
		for i := range vs {
			var err error
			vs[i], rd, err = f.decode(rd, t.Elem, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, nil, err
			}
		}
		return vs, rest, nil
	}
	return nil, nil, fmt.Errorf("%s: unknown kind %d", path, t.Kind)
}
