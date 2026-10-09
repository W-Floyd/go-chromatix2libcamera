package chromatix

import (
	"encoding/json"
	"testing"
)

func TestGeneric(t *testing.T) {
	b := &builder{}
	// Two elements of three slots each, and a string.
	items := b.add("mod_x_data", 0, NoGroup, slots(1, fbits(1.5), 7, 2, fbits(-0.25), 8))
	text := b.add("mod_x_name", 0, NoGroup, []byte("hello\x00"))
	mod := b.add("x01_ope", 1|1<<16, 4, slots(
		5,          // s0: an integer
		fbits(0.5), // s1: a float
		1<<32|2,    // s2: two packed integers
		2, items,   // s3-s4: a reference to two elements
		6, text, // s5-s6: a string
		0, // s7
	))

	f, err := Parse(b.bytes())
	if err != nil {
		t.Fatal(err)
	}
	s, _ := f.Symbol(mod)
	got, err := json.Marshal(f.Generic(s))
	if err != nil {
		t.Fatal(err)
	}

	want := `{"s0":5,"s1":0.5,"s2":[2,1],` +
		`"s3":{"ref":"mod_x_data","count":2,"items":[` +
		`{"s0":1,"s1":1.5,"s2":7},{"s0":2,"s1":-0.25,"s2":8}]},` +
		`"s5":{"ref":"mod_x_name","count":6,"string":"hello"},"s7":0}`
	if string(got) != want {
		t.Errorf("Generic:\n got %s\nwant %s", got, want)
	}
}

func TestGuessNumber(t *testing.T) {
	for _, c := range []struct {
		w    uint32
		want Value
	}{
		{0, int64(0)},
		{5, int64(5)},
		{0xffffffff, int64(-1)},
		{0x3f800000, GuessedFloat(1.0)},
		{0xbf800000, GuessedFloat(-1.0)},
		{100000000, int64(100000000)},
	} {
		if got := guessNumber(c.w); got != c.want {
			t.Errorf("guessNumber(%#x) = %v, want %v", c.w, got, c.want)
		}
	}
}

// Strings and packed lists that do not fill whole slots are padded to them.
func TestGenericPadded(t *testing.T) {
	b := &builder{}
	name := b.add("mod_x_name", 0, NoGroup, append([]byte("EnableDepth\x00"), make([]byte, 4)...)) // 12 characters, 16 bytes
	nums := b.add("mod_x_nums", 0, NoGroup, slots(1<<32|2, 3))                                     // 3 numbers, 16 bytes
	mod := b.add("x01_ope", 1|1<<16, 4, slots(12, name, 3, nums, fbits(900)))

	f, err := Parse(b.bytes())
	if err != nil {
		t.Fatal(err)
	}
	s, _ := f.Symbol(mod)
	got, err := json.Marshal(f.Generic(s))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"s0":{"ref":"mod_x_name","count":12,"string":"EnableDepth"},` +
		`"s2":{"ref":"mod_x_nums","count":3,"items":[2,1,3]},"s4":900.0}`
	if string(got) != want {
		t.Errorf("Generic:\n got %s\nwant %s", got, want)
	}
}
