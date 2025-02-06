package types

import "github.com/golang/geo/r3"

// Triangle represents a 3D triangle with three vertices
type Triangle struct {
	V1, V2, V3 r3.Vector
}
