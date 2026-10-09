package chromatix

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Most of a tuning file's modules have no public schema (the file names
// their elements, but not what a module's fields are, which Qualcomm's XSDs
// define). Generic decodes such a module without one: all of what it holds,
// with fields named by their slot, so that nothing the vendor tuned is out of
// reach even where nothing yet knows what it means.
//
// What the format fixes is enough to walk a module. Its data is a run of
// 8-byte slots (see slot), each a 32-bit value in the low half, except that
// numbers of a fixed-length list are packed two to a slot, and a (count,
// symbol id) pair of slots refers to another symbol, whose data is count
// elements (or characters) back to back. What the format does not say is how
// a slot is typed or whether it is one, so it is guessed, from the value:
//
//   - a (count, id) pair is taken for a reference where id is a symbol and
//     count elements fill the symbol's data;
//   - a slot with something in its upper half is two packed numbers;
//   - a number is a float where its bit pattern is one of a plausible
//     magnitude (1e-9 to 1e19), and an integer otherwise.
//
// A wrong guess misleads about one value, never about where the next is, and
// the guess is stated: floats are written with a decimal point (900.0),
// integers without.

// maxGenericDepth is how deep references are followed; the files' nesting is
// a few levels.
const maxGenericDepth = 8

// Generic decodes a symbol's data without a schema. The result is an *Object
// whose fields are the slots, named "s<index>", where a slot that starts a
// reference is an *Object with the referenced symbol, its element count and
// its elements, and the slot after it is left out.
func (f *File) Generic(s Symbol) Value {
	return f.generic(f.Data(s), 0, map[uint64]bool{s.ID: true})
}

func (f *File) generic(d []byte, depth int, path map[uint64]bool) Value {
	le := binary.LittleEndian
	o := &Object{}
	n := len(d) / slot

	for i := 0; i < n; i++ {
		name := "s" + strconv.Itoa(i)

		// A reference takes this slot and the next.
		if i+1 < n && depth < maxGenericDepth {
			if v, ok := f.genericRef(d[i*slot:], depth, path); ok {
				o.Names = append(o.Names, name)
				o.Values = append(o.Values, v)
				i++
				continue
			}
		}

		lo, hi := le.Uint32(d[i*slot:]), le.Uint32(d[i*slot+4:])
		var v Value
		if hi != 0 {
			v = []Value{guessNumber(lo), guessNumber(hi)}
		} else {
			v = guessNumber(lo)
		}
		o.Names = append(o.Names, name)
		o.Values = append(o.Values, v)
	}

	// Data that is not a whole number of slots: a packed list's tail.
	if rest := d[n*slot:]; len(rest) > 0 {
		var vs []Value
		for len(rest) >= 4 {
			vs = append(vs, guessNumber(le.Uint32(rest)))
			rest = rest[4:]
		}
		o.Names = append(o.Names, "tail")
		o.Values = append(o.Values, vs)
	}
	return o
}

// genericRef decodes a (count, id) pair at the start of d, if that is what it
// is.
func (f *File) genericRef(d []byte, depth int, path map[uint64]bool) (Value, bool) {
	le := binary.LittleEndian
	count, id := le.Uint64(d), le.Uint64(d[slot:])
	if count == 0 || count > 1<<20 || id == 0 || path[id] {
		return nil, false
	}
	ref, ok := f.Symbol(id)
	if !ok || ref.Size == 0 || ref.IsModule() {
		return nil, false
	}
	rd := f.Data(ref)
	size := uint64(len(rd))

	// A list of characters or packed numbers that does not fill whole slots
	// is padded to them, which is not an element size.
	padded := func(unit uint64) bool { return count*unit <= size && (count*unit+slot-1)/slot*slot == size }
	var elem uint64
	switch {
	case size >= count && size%count == 0:
		elem = size / count
	case padded(1) || padded(4):
		elem = 0
	default:
		return nil, false
	}

	o := &Object{
		Names:  []string{"ref", "count"},
		Values: []Value{ref.Name, count},
	}

	// Characters.
	if elem == 1 || ((elem == 0 || elem%slot != 0) && padded(1)) {
		if s, ok := printable(rd[:count]); ok {
			o.Names = append(o.Names, "string")
			o.Values = append(o.Values, s)
			return o, true
		}
	}

	path[id] = true
	defer delete(path, id)

	var items []Value
	switch {
	case elem != 0 && elem%slot == 0:
		for k := uint64(0); k < count; k++ {
			items = append(items, f.generic(rd[k*elem:(k+1)*elem], depth+1, path))
		}
	case elem == 0 && padded(4):
		for k := uint64(0); k < count; k++ {
			items = append(items, guessNumber(le.Uint32(rd[k*4:])))
		}
	case elem == 0:
		return nil, false
	case elem%4 == 0:
		// Packed numbers.
		for k := uint64(0); k < uint64(len(rd))/4; k++ {
			items = append(items, guessNumber(le.Uint32(rd[k*4:])))
		}
	default:
		items = append(items, fmt.Sprintf("%x", rd))
	}
	o.Names = append(o.Names, "items")
	o.Values = append(o.Values, items)
	return o, true
}

// GuessedFloat is a float that guessNumber found, written with a decimal point
// even when it is whole, so that the output tells it from an integer.
type GuessedFloat float64

func (x GuessedFloat) MarshalJSON() ([]byte, error) {
	s := strconv.FormatFloat(float64(x), 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return []byte(s), nil
}

// guessNumber types a 32-bit value: a float if its bits are a float of a
// plausible magnitude, an integer (signed) otherwise.
func guessNumber(w uint32) Value {
	if w == 0 {
		return int64(0)
	}
	if a := w &^ (1 << 31); a >= 0x30800000 && a < 0x5f800000 {
		// Rounded to what a float32 holds, for output that is not all digits.
		x := float64(math.Float32frombits(w))
		r, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'g', 7, 32), 64)
		return GuessedFloat(r)
	}
	return int64(int32(w))
}

// printable returns b as a string, if it is a NUL-terminated string of
// printable characters (or such characters alone).
func printable(b []byte) (string, bool) {
	end := len(b)
	if end > 0 && b[end-1] == 0 {
		end--
	}
	if end == 0 {
		return "", false
	}
	for _, c := range b[:end] {
		if c < 0x20 || c > 0x7e {
			return "", false
		}
	}
	return string(b[:end]), true
}
