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

		var f *os.File
		f, err = os.Open(assetPath + "/" + file.Name())
		if err != nil {
			log.Errorf("Error opening texture file %s: %v", file.Name(), err)
			continue
		}

		var img image.Image
		img, _, err = image.Decode(f)
		err = f.Close()
		if err != nil {
			log.Errorf("Failed to close image: %v", err)
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

	log.Debugf("Texture Atlas loaded into memory. (Size limit: %dpx)", textureSizeLimit)
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

func blitYUV(canvas []uint8, tex *entities.Texture, startX, startY, canvasW, uOff, vOff int) {
	strideY := canvasW
	strideUV := canvasW / 2

	rows := tex.Rect.Dy()
	if maxRows := uOff/strideY - startY; maxRows < rows {
		rows = maxRows
	}
	cols := tex.Rect.Dx()
	if maxCols := canvasW - startX; maxCols < cols {
		cols = maxCols
	}
	if rows <= 0 || cols <= 0 {
		return
	}

	for row := 0; row < rows; row++ {
		ty := startY + row
		rowOffset := ty * strideY
		texRowOffset := row * tex.Stride
		evenRow := ty%2 == 0
		uvRowOffset := (ty / 2) * strideUV

		for col := 0; col < cols; col++ {
			tIdx := texRowOffset + col*4
			r, g, b := tex.Pix[tIdx], tex.Pix[tIdx+1], tex.Pix[tIdx+2]

			yVal := uint8((66*int(r)+129*int(g)+25*int(b)+128)>>8 + 16)
			canvas[rowOffset+startX+col] = yVal

			if evenRow && (startX+col)%2 == 0 {
				uVal := uint8((-38*int(r)-74*int(g)+112*int(b)+128)>>8 + 128)
				vVal := uint8((112*int(r)-94*int(g)-18*int(b)+128)>>8 + 128)

				uvIdx := uvRowOffset + (startX+col)/2
				canvas[uOff+uvIdx] = uVal
				canvas[vOff+uvIdx] = vVal
			}
		}
	}
}
