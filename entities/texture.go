package entities

import "image"

type Texture struct {
	Pix    []byte
	Stride int
	Rect   image.Rectangle
}
