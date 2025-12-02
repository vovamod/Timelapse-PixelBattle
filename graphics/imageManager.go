package graphics

import (
	"Timelapse-PixelBattle/common"
	"fmt"
	"image"
	"image/png"
	"os"
	"sync"

	"github.com/vovamod/utils/log"

	"github.com/fogleman/gg"
)

var textureCache sync.Map

// NON cached/buffered func-s
func loadImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func(file *os.File) {
		err = file.Close()
		if err != nil {
			log.Info("Error closing file: ", err)
		}
	}(file)
	var img image.Image
	img, err = png.Decode(file)
	if err != nil {
		return nil, err
	}
	return img, nil
}

// imageToRGB converts an image to RGB24 byte array (no alpha)
func imageToRGB(img image.Image, imType int, buffer *[]byte) {
	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y

	if len(*buffer) != 0 {
		var iBuf []byte
		iBuf = *buffer
		index := 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				iBuf[index] = uint8(r >> 8)
				iBuf[index+1] = uint8(g >> 8)
				iBuf[index+2] = uint8(b >> 8)
				index += 3
			}
		}
		*buffer = iBuf
		return
	}

	if imType == 24 {
		rgb := make([]byte, width*height*4) // 4 bytes per pixel (RGBA)
		index := 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				// Convert from uint32 to uint8 (maybe... there IS a better way)
				rgb[index] = uint8(r >> 8)
				rgb[index+1] = uint8(g >> 8)
				rgb[index+2] = uint8(b >> 8)
				rgb[index+3] = uint8(a >> 8)
				index += 4
			}
		}
		*buffer = rgb
		return
	}

	rgb := make([]byte, width*height*3) // 3 byte per pixel
	index := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			rgb[index] = uint8(r >> 8)
			rgb[index+1] = uint8(g >> 8)
			rgb[index+2] = uint8(b >> 8)
			index += 3
		}
	}
	*buffer = rgb
	return
}

// Wrapper on top of loadImage
func getCachedImage(path string) (*image.Image, error) {
	// Check if texture is already cached
	if tex, exists := textureCache.Load(path); exists {
		i := tex.(image.Image)
		return &i, nil
	}

	// Load the texture
	tex, err := loadImage(path)
	if err != nil {
		return nil, err
	}

	// Store it in the cache
	textureCache.Store(path, tex)
	return &tex, nil
}

func frameCreate(blocks *[]common.VisualData, im *image.Image, width, height, textureSize int, frameBuffer *[]byte) image.Image {
	var dc *gg.Context
	if im != nil {
		dc = gg.NewContextForImage(*im)
	} else {
		texture, err := getCachedImage("assets/white_concrete.png")
		if err != nil {
			log.Warn(fmt.Sprintf("Could not load background texture: %v", err))
			dc = gg.NewContext(width, height)
			dc.SetRGB(1, 1, 1)
			dc.Clear()
		} else {
			dc = gg.NewContext(width, height)
			for x := 0; x < width; x += textureSize {
				for y := 0; y < height; y += textureSize {
					dc.DrawImage(*texture, x, y)
				}
			}
		}
	}

	textureSize64 := int64(textureSize)
	for _, block := range *blocks {
		texture, err := getCachedImage("assets/" + block.BlockTexture)
		if err != nil {
			log.Warn(fmt.Sprintf("Error loading texture %s: %v", block.BlockTexture, err))
			continue
		}
		dc.DrawImage(*texture, int(block.X*textureSize64), int(block.Y*textureSize64))
	}
	img := dc.Image()
	// buffered
	if frameBuffer != nil {
		imageToRGB(img, 0, frameBuffer)
	}
	return dc.Image()
}

func removeOldData(data *[]common.VisualData) {
	if data == nil || len(*data) == 0 {
		return
	}

	latest := make(map[string]common.VisualData, len(*data))
	for _, v := range *data {
		key := fmt.Sprintf("%d:%d", v.X, v.Y)
		if prev, ok := latest[key]; !ok || v.Time.After(prev.Time) {
			latest[key] = v
		}
	}

	write := 0
	seen := make(map[string]bool, len(latest))
	for _, v := range *data {
		key := fmt.Sprintf("%d:%d", v.X, v.Y)
		if seen[key] {
			continue
		}
		if newest, ok := latest[key]; ok && newest.Time.Equal(v.Time) {
			(*data)[write] = v
			write++
			seen[key] = true
		}
	}
	*data = (*data)[:write]
}
