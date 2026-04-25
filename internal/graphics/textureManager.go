package graphics

import (
	"Timelapse-PixelBattle/pkg/entities"
	"image"
	"image/draw"
	"os"
	"strings"
	"sync"

	"github.com/vovamod/utils/log"
)

var textureCacheRaw sync.Map

func LoadTextureAtlas(assetPath string, textureSizeLimit int) error {
	files, err := os.ReadDir(assetPath)
	if err != nil {
		return err
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".png") {
			continue
		}

		f, err := os.Open(assetPath + "/" + file.Name())
		if err != nil {
			log.Errorf("Error opening texture file %s: %v", file.Name(), err)
			continue
		}

		img, _, err := image.Decode(f)
		if err != nil {
			log.Errorf("Failed to decode %s: %v", file.Name(), err)
			continue
		}
		err = f.Close()
		if err != nil {
			log.Errorf("Failed to close file %s: %v", file.Name(), err)
			continue
		}

		bounds := img.Bounds()
		origWidth := bounds.Dx()

		finalSize := origWidth
		if textureSizeLimit > 0 && textureSizeLimit < origWidth {
			finalSize = textureSizeLimit
		}

		var finalImg *image.RGBA

		if finalSize == origWidth {
			rgba := image.NewRGBA(bounds)
			draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
			finalImg = rgba
		} else {
			finalImg = image.NewRGBA(image.Rect(0, 0, finalSize, finalSize))
			draw.Draw(finalImg, finalImg.Bounds(), img, bounds.Min, draw.Src)
		}

		textureCacheRaw.Store(file.Name(), &entities.Texture{
			Pix:    finalImg.Pix,
			Stride: finalImg.Stride,
			Rect:   finalImg.Bounds(),
		})
	}

	log.Successf("Texture Atlas loaded into memory. (Size limit: %dpx)", textureSizeLimit)
	return nil
}

func getRawTexture(name string) (*entities.Texture, bool) {
	if val, ok := textureCacheRaw.Load(name); ok {
		return val.(*entities.Texture), true
	}
	return nil, false
}

func fastBlit(canvas *image.RGBA, tex *entities.Texture, x, y int) {

	// OOB fail safe 1
	rect := tex.Rect.Add(image.Pt(x, y)).Intersect(canvas.Bounds())
	if rect.Empty() {
		return
	}
	localX := rect.Min.X - x
	localY := rect.Min.Y - y
	paintWidth := rect.Dx() * 4

	for row := 0; row < tex.Rect.Dy(); row++ {
		canvasOffset := (rect.Min.Y+row)*canvas.Stride + (rect.Min.X * 4)
		texOffset := (localY+row)*tex.Stride + (localX * 4)

		// OOB fail safe 2
		copy(canvas.Pix[canvasOffset:canvasOffset+paintWidth],
			tex.Pix[texOffset:texOffset+paintWidth])
	}
}
