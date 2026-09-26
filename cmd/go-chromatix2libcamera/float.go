package main

import "math"

func float32FromBits(v uint32) float32 {
	return math.Float32frombits(v)
}

// isPlausibleFloat is a display heuristic: a 32-bit word that decodes to a
// normal float of moderate magnitude is more likely a float than an
// integer, since small integers decode as denormals.
func isPlausibleFloat(f float32) bool {
	a := math.Abs(float64(f))
	return !math.IsNaN(a) && !math.IsInf(a, 0) && a >= 1e-6 && a <= 1e7
}
