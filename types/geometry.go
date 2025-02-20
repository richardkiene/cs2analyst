package types

import (
	"math"

	"github.com/golang/geo/r3"
)

// Triangle represents a 3D triangle with three vertices
type Triangle struct {
	V1, V2, V3 r3.Vector
}

// Equals checks if two triangles have the same vertices (within a small epsilon).
func (t Triangle) Equals(other Triangle) bool {
	const eps = 0.000001
	return vectorsApproxEqual(t.V1, other.V1, eps) &&
		vectorsApproxEqual(t.V2, other.V2, eps) &&
		vectorsApproxEqual(t.V3, other.V3, eps)
}

func vectorsApproxEqual(a, b r3.Vector, eps float64) bool {
	return math.Abs(a.X-b.X) < eps &&
		math.Abs(a.Y-b.Y) < eps &&
		math.Abs(a.Z-b.Z) < eps
}
