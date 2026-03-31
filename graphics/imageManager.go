package graphics

import (
	"Timelapse-PixelBattle/common"
	"fmt"
	"image"
	"image/draw"
	"os"
	"strings"
	"sync"

	"github.com/vovamod/utils/log"
)

var textureCacheRaw sync.Map

func LoadTextureAtlas(assetPath string) error {
	files, err := os.ReadDir(assetPath)
	if err != nil {
		return err
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".png") {
			continue
		}
		var f *os.File
		var img image.Image
		f, err = os.Open(assetPath + "/" + file.Name())
		if err != nil {
			continue
		}

		img, _, err = image.Decode(f)
		if err != nil {
			log.Errorf("Failed to decode %s: %v", file.Name(), err)
			continue
		}
		err = f.Close()
		if err != nil {
			log.Errorf("Failed to close %s: %v", file.Name(), err)
			return err
		}

		bounds := img.Bounds()
		rgba := image.NewRGBA(bounds)
		draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

		textureCacheRaw.Store(file.Name(), &common.Texture{
			Pix:    rgba.Pix,
			Stride: rgba.Stride,
			Rect:   bounds,
		})
	}
	log.Success("Texture Atlas loaded into memory.")
	return nil
}

func getRawTexture(name string) (*common.Texture, bool) {
	if val, ok := textureCacheRaw.Load(name); ok {
		return val.(*common.Texture), true
	}
	return nil, false
}

func fastBlit(canvas *image.RGBA, tex *common.Texture, x, y int) {
	paintWidth := tex.Rect.Dx() * 4 // 4 bytes (RGBA)

	for row := 0; row < tex.Rect.Dy(); row++ {
		canvasOffset := (y+row)*canvas.Stride + (x * 4)
		texOffset := row * tex.Stride

		// OOB CHECK U DUMBSHIT
		if canvasOffset+paintWidth <= len(canvas.Pix) {
			copy(canvas.Pix[canvasOffset:canvasOffset+paintWidth], tex.Pix[texOffset:texOffset+paintWidth])
		}
	}
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
