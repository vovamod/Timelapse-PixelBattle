package graphics

import (
	"Timelapse-PixelBattle/pkg/entities"
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/vovamod/utils/log"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

func EncodeGPU(dest []entities.VisualData, width, height, iterations, textureSize, framerate int,
	filename, playername string, eGPU entities.GPUSelection, renderTime, debug bool, onPreview func(img image.Image, progress float64)) error {
	uiOffset := 0
	if renderTime {
		uiOffset = height / 10
		if uiOffset < 40 {
			uiOffset = 40
		}
	}
	lenght := len(dest)
	log.Info(fmt.Sprintf("Rendering graphics data for %d elements ", lenght))
	log.Info(fmt.Sprintf("Current configuration:\n  - Width: %v\n  - Height: %v\n  - Iterations: %v\n  - TextureSize: %v\n  - Framerate: %v",
		width, height, iterations, textureSize, framerate))

	needsScaling := (width > 3840 || height > 2160) && eGPU.EncoderName != "libx264" // ye. we need to keep in mind that anything other than x264 (CPU) encoders have limits

	if needsScaling {
		scaledWidth, scaledHeight := calculateScaledDimensions(width, height, eGPU.GPUType)
		log.Info(fmt.Sprintf("Output resolution (will be scaled by ffmpeg): %dx%d", scaledWidth, scaledHeight))
	}

	inputHeight := height + uiOffset
	strideY := width
	strideUV := width / 2
	uOffset := inputHeight * strideY
	vOffset := uOffset + (inputHeight/2)*strideUV
	totalSize := (inputHeight * width * 3) / 2
	outputArgs := getEncoderArgs(eGPU, width, inputHeight, needsScaling)

	pr, pw := io.Pipe()
	bufferPool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, totalSize)
		},
	}
	pwBuffered := bufio.NewWriterSize(pw, 1024*1024*64) // for myself: Use between 2 MB - 78 MB. Anything else -> trash
	frameChan := make(chan []uint8, 60)
	errChan := make(chan error, 1)
	go func() {
		for pix := range frameChan {
			if _, err := pwBuffered.Write(pix); err != nil {
				errChan <- err
				return
			}
			bufferPool.Put(pix)
		}
		err := pwBuffered.Flush()
		if err != nil {
			log.Warn("Error flushing buffered writer")
		}
		err = pw.Close()
		if err != nil {
			log.Warn("Error closing buffered writer")
		}
	}()
	go func() {
		var err error
		if debug {
			err = ffmpeg.Input("pipe:0", ffmpeg.KwArgs{
				"f":                 "rawvideo",
				"pix_fmt":           "yuv420p",
				"s":                 fmt.Sprintf("%dx%d", width, inputHeight),
				"r":                 fmt.Sprintf("%d", framerate),
				"thread_queue_size": "8192", // Doubled again
				"threads":           "4",
			}).
				Output(filename, outputArgs).
				Silent(false).
				WithInput(pr).
				ErrorToStdOut().
				OverWriteOutput().
				Run()
		} else {
			err = ffmpeg.Input("pipe:0", ffmpeg.KwArgs{
				"f":                 "rawvideo",
				"pix_fmt":           "yuv420p",
				"s":                 fmt.Sprintf("%dx%d", width, inputHeight),
				"r":                 fmt.Sprintf("%d", framerate),
				"thread_queue_size": "8192", // Doubled again
				"threads":           "4",
			}).
				Output(filename, outputArgs).
				OverWriteOutput().
				WithInput(pr).
				Run()
		}
		errChan <- err
	}()

	masterCanvas := bufferPool.Get().([]uint8)
	for i := 0; i < uOffset; i++ {
		masterCanvas[i] = 235
	}
	for i := uOffset; i < len(masterCanvas); i++ {
		masterCanvas[i] = 128
	}
	//bgTex, ok := getRawTexture("white_concrete.png")
	//if ok {
	//	texW, texH := bgTex.Rect.Dx(), bgTex.Rect.Dy()
	//	for y := 0; y < height; y += texH {
	//		for x := 0; x < width; x += texW {
	//			blitYUV(masterCanvas, bgTex, x, y, width, uOffset, vOffset)
	//		}
	//	}
	//}
	batchSize := iterations
	totalFrames := (lenght + batchSize - 1) / batchSize

	// GUI
	previewInterval := totalFrames / 20
	if previewInterval < 1 {
		previewInterval = 1
	}

	for i := 0; i < lenght; i += batchSize {
		end := i + batchSize
		if end > lenght {
			end = lenght
		}
		batch := dest[i:end]
		renderTimer := time.Now()

		for _, block := range batch {
			tex, ok := getRawTexture(block.BlockTexture)
			if !ok {
				continue
			}
			targetX := int(block.X) * textureSize
			targetY := int(block.Y) * textureSize
			blitYUV(masterCanvas, tex, targetX, targetY, width, uOffset, vOffset)
		}

		if renderTime {
			currentFrame := (i / batchSize) + 1
			ts := batch[len(batch)-1].Time.Format("2006-01-02 15:04")
			drawFooterYUV(masterCanvas, width, height, uiOffset, currentFrame, ts, playername)
		}
		log.Debugf("Frame prepared: %v", time.Since(renderTimer))
		toPipe := bufferPool.Get().([]uint8)
		copy(toPipe, masterCanvas)
		pipeTimer := time.Now()
		select {
		case frameChan <- toPipe:
		case ffmpegErr := <-errChan:
			return fmt.Errorf("ffmpeg crashed: %v", ffmpegErr)
		}

		// GUI
		if onPreview != nil {
			currentFrameIdx := i / batchSize
			progressPercent := float64(currentFrameIdx+1) / float64(totalFrames)
			if currentFrameIdx%previewInterval == 0 || currentFrameIdx == totalFrames-1 {
				onPreview(convertYUVToImage(masterCanvas, width, inputHeight, uOffset, vOffset), progressPercent)
			} else {
				onPreview(nil, progressPercent)
			}
		}

		log.Debugf("Pipe Write: %v", time.Since(pipeTimer))
		log.CustomStreamf("info", "Progress: %d/%d frames", (i/batchSize)+1, totalFrames)
	}
	close(frameChan)
	ffmpegErr := <-errChan
	if ffmpegErr != nil {
		log.Errorf("FFmpeg finished with error: %v", ffmpegErr)
	}
	VerifyVideoFile(filename)
	return nil
}

func GeneratePhotoLocal(dest *[]entities.VisualData, width, height, textureSize int, filename string) (image.Image, error) {
	log.Info(fmt.Sprintf("Generating high-res photo:\n  - Resolution: %dx%d\n  - Texture Size: %v", width, height, textureSize))

	canvas := image.NewRGBA(image.Rect(0, 0, width, height))

	for i := 0; i < len(canvas.Pix); i++ {
		canvas.Pix[i] = 255
	}

	// MINE
	//bgTex, ok := getRawTexture("white_concrete.png")
	//if ok {
	//	for y := 0; y < height; y += textureSize {
	//		for x := 0; x < width; x += textureSize {
	//			fastBlit(canvas, bgTex, x, y)
	//		}
	//	}
	//}

	start := time.Now()
	for _, block := range *dest {
		tex, ok := getRawTexture(block.BlockTexture)
		if !ok {
			continue
		}

		posX := int(block.X) * textureSize
		posY := int(block.Y) * textureSize

		fastBlit(canvas, tex, posX, posY)
	}
	log.Successf("Canvas rendered in %v", time.Since(start))

	f, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("could not create file: %w", err)
	}
	defer func(f *os.File) {
		err = f.Close()
		if err != nil {
			log.Errorf("Error during file closure: %v", err.Error())
		}
	}(f)

	if err = png.Encode(f, canvas); err != nil {
		return nil, fmt.Errorf("png encoding failed: %w", err)
	}

	log.Successf("Photo saved to: %s", filename)
	return canvas, nil
}

func VerifyVideoFile(filename string) {
	log.Notice(fmt.Sprintf("Running ffprobe verification on %s", filename))
	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,nb_frames,codec_name",
		"-of", "default=noprint_wrappers=1",
		filename,
	}

	cmd := exec.Command("ffprobe", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Errorf("Verification Failed! ffprobe reported an error: %v", err)
		log.Errorf("ffprobe output: %s", string(output))
		return
	}

	stats := strings.ReplaceAll(string(output), "\n", " | ")
	log.Successf("Video Verified: %s", stats)
}

// Other func

func convertYUVToImage(yuv []byte, w, h, uOff, vOff int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			yIdx := y*w + x
			uvIdx := (y/2)*(w/2) + (x / 2)

			Y := float64(yuv[yIdx])
			U := float64(yuv[uOff+uvIdx]) - 128
			V := float64(yuv[vOff+uvIdx]) - 128

			r := Y + 1.402*V
			g := Y - 0.344136*U - 0.714136*V
			b := Y + 1.772*U

			img.Set(x, y, color.RGBA{
				R: uint8(clamp(r)),
				G: uint8(clamp(g)),
				B: uint8(clamp(b)),
				A: 255,
			})
		}
	}
	return img
}

func clamp(val float64) float64 {
	if val < 0 {
		return 0
	}
	if val > 255 {
		return 255
	}
	return val
}

func drawFooterYUV(pix []uint8, w, h, uiH, frame int, timestamp string, playername string) {
	strideY := w
	strideUV := w / 2

	for row := h; row < h+uiH; row++ {
		for col := 0; col < w; col++ {
			idx := row*strideY + col
			if idx < len(pix) {
				pix[idx] = 35
			}
		}
	}
	totalHeight := h + uiH
	uOffset := totalHeight * strideY
	vOffset := uOffset + (totalHeight/2)*strideUV
	footerStartUV := h / 2
	footerHeightUV := uiH / 2
	for row := footerStartUV; row < footerStartUV+footerHeightUV; row++ {
		for col := 0; col < strideUV; col++ {
			uvIdx := row*strideUV + col
			if uOffset+uvIdx < vOffset {
				pix[uOffset+uvIdx] = 128
			}
			if vOffset+uvIdx < len(pix) {
				pix[vOffset+uvIdx] = 128
			}
		}
	}

	scale := uiH / 25
	if scale < 1 {
		scale = 1
	}

	leftText := fmt.Sprintf("FRAME: %d", frame)
	rightText := timestamp
	centerText := "PIXEL BATTLE TIMELAPSE"
	if playername != "" {
		centerText = fmt.Sprintf("PLAYER: %s", playername)
	}

	padding := w / 50
	textHeight := 13 * scale
	textY := h + (uiH / 2) - (textHeight / 2)

	addSimpleTextYUV(pix, padding, textY, leftText, w, strideY, scale)
	rWidth := getTextWidth(rightText, scale)
	addSimpleTextYUV(pix, w-rWidth-padding, textY, rightText, w, strideY, scale)
	cWidth := getTextWidth(centerText, scale)
	addSimpleTextYUV(pix, (w/2)-(cWidth/2), textY, centerText, w, strideY, scale)
}

func addSimpleTextYUV(pix []uint8, x, y int, label string, w, strideY, scale int) {
	face := basicfont.Face7x13
	ascent := 11
	dot := fixed.Point26_6{
		X: fixed.Int26_6(x << 6),
		Y: fixed.Int26_6((y + (ascent * scale / 8)) << 6),
	}

	yLimit := len(pix)
	if strideY > 0 {
	}

	for _, char := range label {
		dr, mask, maskp, advance, ok := face.Glyph(dot, char)
		if !ok {
			continue
		}

		for my := 0; my < dr.Dy(); my++ {
			for mx := 0; mx < dr.Dx(); mx++ {
				_, _, _, a := mask.At(maskp.X+mx, maskp.Y+my).RGBA()
				if a > 0 {
					for sy := 0; sy < scale; sy++ {
						for sx := 0; sx < scale; sx++ {
							px := dr.Min.X + (mx * scale) + sx
							py := dr.Min.Y + (my * scale) + sy

							if px >= 0 && px < w && py >= 0 {
								idx := py*strideY + px
								if idx < yLimit {
									pix[idx] = 255
								}
							}
						}
					}
				}
			}
		}
		dot.X += advance * fixed.Int26_6(scale)
	}
}

func getTextWidth(label string, scale int) int {
	face := basicfont.Face7x13
	totalWidth := 0
	for _, char := range label {
		_, _, _, advance, ok := face.Glyph(fixed.Point26_6{}, char)
		if !ok {
			continue
		}
		totalWidth += (int(advance) >> 6) * scale
	}
	return totalWidth
}
