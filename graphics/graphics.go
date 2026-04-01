package graphics

import (
	"Timelapse-PixelBattle/entities"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/vovamod/utils/log"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

func EncodeGPU(dest []entities.VisualData, width, height, iterations, textureSize, framerate int, filename string, playername *string, renderTime, debug bool) error {
	uiOffset := 0
	if renderTime {
		uiOffset = 200
	}
	log.Info(fmt.Sprintf("Rendering graphics data for %d elements with GPU-optimized frames", len(dest)))
	log.Info(fmt.Sprintf("Current configuration:\n  - Width: %v\n  - Height: %v\n  - Iterations: %v\n  - TextureSize: %v\n  - Framerate: %v",
		width, height, iterations, textureSize, framerate))

	encoder, encoderName, gpuType := getGPUEncoder(width, height)
	log.Info(fmt.Sprintf("Selected encoder: %s (%s) for %s", encoderName, encoder, gpuType))

	needsScaling := (width > 3840 || height > 2160) && encoderName != "libx264" // ye. we need to keep in mind that anything other than x264 (CPU) encoders have limits

	if needsScaling {
		scaledWidth, scaledHeight := calculateScaledDimensions(width, height, gpuType)
		log.Info(fmt.Sprintf("Output resolution (will be scaled by ffmpeg): %dx%d", scaledWidth, scaledHeight))
	}

	inputHeight := height + uiOffset
	outputArgs := getEncoderArgs(encoder, encoderName, gpuType, width, inputHeight, needsScaling)
	// add pipe
	pr, pw := io.Pipe()

	stride := width * 3
	pix := make([]uint8, inputHeight*stride)
	for i := range pix {
		pix[i] = 255
	}

	errChan := make(chan error, 1)
	go func() {
		var err error
		if debug {
			err = ffmpeg.Input("pipe:0", ffmpeg.KwArgs{
				"f":                 "rawvideo",
				"pix_fmt":           "rgb24",
				"s":                 fmt.Sprintf("%dx%d", width, inputHeight),
				"r":                 fmt.Sprintf("%d", framerate),
				"thread_queue_size": "1024", // Buffer for high-speed input
			}).
				Output(filename, outputArgs).
				Silent(false).
				WithInput(pr).
				OverWriteOutput().
				Run()
		} else {
			err = ffmpeg.Input("pipe:0", ffmpeg.KwArgs{
				"f":                 "rawvideo",
				"pix_fmt":           "rgb24",
				"s":                 fmt.Sprintf("%dx%d", width, inputHeight),
				"r":                 fmt.Sprintf("%d", framerate),
				"thread_queue_size": "1024", // Buffer for high-speed input
			}).
				Output(filename, outputArgs).
				OverWriteOutput().
				WithInput(pr).
				Run()
		}
		errChan <- err
	}()

	batchSize := iterations
	totalFrames := (len(dest) + batchSize - 1) / batchSize

	for i := 0; i < len(dest); i += batchSize {
		end := i + batchSize
		if end > len(dest) {
			end = len(dest)
		}
		batch := dest[i:end]

		renderTimer := time.Now()
		for _, block := range batch {
			tex, ok := getRawTexture(block.BlockTexture)
			if !ok {
				continue
			}
			// Convert RGBA -> RGB24 (255,255,255)
			targetX := int(block.X) * textureSize
			targetY := int(block.Y) * textureSize

			texWidth := tex.Rect.Dx()
			texHeight := tex.Rect.Dy()

			for row := 0; row < texHeight; row++ {
				canvasRowStart := (targetY+row)*stride + (targetX * 3)
				texRowStart := row * tex.Stride

				// SB check
				if canvasRowStart >= 0 && canvasRowStart+(texWidth*3) <= len(pix) {
					for col := 0; col < texWidth; col++ {
						cIdx := canvasRowStart + (col * 3)
						tIdx := texRowStart + (col * 4)

						// Copy R, G, B
						pix[cIdx] = tex.Pix[tIdx]
						pix[cIdx+1] = tex.Pix[tIdx+1]
						pix[cIdx+2] = tex.Pix[tIdx+2]
					}
				}
			}
		}
		if renderTime {
			currentFrame := (i / batchSize) + 1
			ts := batch[len(batch)-1].Time.Format("2006-01-02 15:04")

			drawFooter(pix, width, height, uiOffset, currentFrame, ts, playername)
		}

		log.Debugf("Frame prepared: %v", time.Since(renderTimer))

		pipeTimer := time.Now()
		if _, err := pw.Write(pix); err != nil {
			return fmt.Errorf("ffmpeg pipe broken: %w", err)
		}
		log.Debugf("Pipe Write: %v", time.Since(pipeTimer))

		log.CustomStreamf("info", "Progress: %d/%d frames", (i/batchSize)+1, totalFrames)
	}

	err := pw.Close()
	if err != nil {
		log.Errorf("Error while closing pipe: %v", err.Error())
	}
	verifyVideoFile(filename)
	return <-errChan
}

func GeneratePhotoLocal(dest []entities.VisualData, width, height, textureSize int, filename string) error {
	log.Info(fmt.Sprintf("Generating high-res photo:\n  - Resolution: %dx%d\n  - Texture Size: %v", width, height, textureSize))

	canvas := image.NewRGBA(image.Rect(0, 0, width, height))

	start := time.Now()
	for _, block := range dest {
		tex, ok := getRawTexture(block.BlockTexture)
		if !ok {
			log.Infof("Texture %s is missing in assets folder", block.BlockTexture)
			continue
		}

		posX := int(block.X) * textureSize
		posY := int(block.Y) * textureSize

		fastBlit(canvas, tex, posX, posY)
	}
	log.Successf("Canvas rendered in %v", time.Since(start))

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("could not create file: %w", err)
	}
	defer f.Close()

	if err = png.Encode(f, canvas); err != nil {
		return fmt.Errorf("png encoding failed: %w", err)
	}

	log.Successf("Photo saved to: %s", filename)
	return nil
}

// verifyVideoFile uses ffprobe to ensure the GPU encoder produced a valid stream
func verifyVideoFile(filename string) {
	log.Notice(fmt.Sprintf("Running ffprobe verification on %s", filename))
	time.Sleep(5 * time.Second)
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

func drawFooter(pix []uint8, w, h, uiH, frame int, timestamp string, playername *string) {
	stride := w * 3
	scale := 8

	for row := h; row < h+uiH; row++ {
		for col := 0; col < w; col++ {
			idx := row*stride + (col * 3)
			pix[idx], pix[idx+1], pix[idx+2] = 50, 50, 50
		}
	}

	leftText := fmt.Sprintf("FRAME: %d", frame)
	rightText := timestamp
	centerText := "PIXEL BATTLE TIMELAPSE"
	if playername != nil {
		centerText = fmt.Sprintf("PLAYER: %s", *playername)
	}
	padding := w / 50

	textY := h + (uiH / 2) - ((13 * scale) / 2)
	addSimpleText(pix, padding, textY, leftText, w, stride)
	rWidth := getTextWidth(rightText, scale)
	addSimpleText(pix, w-rWidth-padding, textY, rightText, w, stride)
	cWidth := getTextWidth(centerText, scale)
	addSimpleText(pix, (w/2)-(cWidth/2), textY, centerText, w, stride)
}

func addSimpleText(pix []uint8, x, y int, label string, w, stride int) {
	face := basicfont.Face7x13
	scale := 8
	ascent := 11
	dot := fixed.Point26_6{
		X: fixed.Int26_6(x << 6),
		Y: fixed.Int26_6((y + ascent) << 6),
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
								idx := py*stride + (px * 3)
								if idx+2 < len(pix) {
									pix[idx] = 255   // R
									pix[idx+1] = 255 // G
									pix[idx+2] = 255 // B
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
