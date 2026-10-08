package convert

import "testing"

func TestCTWeightCurve(t *testing.T) {
	got := ctWeightCurve([][3]float64{
		{2300, 2300, 0.1}, // a point
		{0, 1300, 0},      // a range, out of order
		{1300, 1300, 0.5}, // a point at a range's end: dropped
		{3000, 3200, 0.25},
	})
	want := []CTWeight{{0, 0}, {1300, 0}, {2300, 0.1}, {3000, 0.25}, {3200, 0.25}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("point %d: got %v, want %v", i, got[i], want[i])
		}
	}
}
