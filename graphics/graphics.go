package graphics

import (
	"Timelapse-PixelBattle/common"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vovamod/utils/log"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// EncodeGPU - Use FFMPEG GPU version
func EncodeGPU(dest []common.VisualData, width, height, iterations, textureSize, framerate int, filename string, debug bool) error {
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

	outputArgs := getEncoderArgs(encoder, encoderName, gpuType, width, height, needsScaling)

	pr, pw := io.Pipe()

	stride := width * 3
	pix := make([]uint8, height*stride)
	for i := range pix {
		pix[i] = 255
	}

	errChan := make(chan error, 1)
	go func() {
		err := ffmpeg.Input("pipe:0", ffmpeg.KwArgs{
			"f":                 "rawvideo",
			"pix_fmt":           "rgb24",
			"s":                 fmt.Sprintf("%dx%d", width, height),
			"r":                 fmt.Sprintf("%d", framerate),
			"thread_queue_size": "1024", // Buffer for high-speed input
		}).
			Output(filename, outputArgs).
			OverWriteOutput().
			Silent(false).
			ErrorToStdOut().
			WithInput(pr).
			Run()
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
		log.Infof("Frame Render: %v", time.Since(renderTimer))

		pipeTimer := time.Now()
		if _, err := pw.Write(pix); err != nil {
			return fmt.Errorf("ffmpeg pipe broken: %w", err)
		}
		log.Infof("Pipe Write: %v", time.Since(pipeTimer))

		if (i/batchSize)%100 == 0 {
			log.Infof("Progress: %d/%d frames", (i/batchSize)+1, totalFrames)
		}
	}

	err := pw.Close()
	if err != nil {
		log.Errorf("Error while closing pipe: %v", err.Error())
	}
	return <-errChan
}

func GeneratePhotoLocal(dest []common.VisualData, width, height, textureSize int, filename string) error {
	log.Info(fmt.Sprintf("Generating high-res photo:\n  - Resolution: %dx%d\n  - Texture Size: %v", width, height, textureSize))

	// 1. Data Cleanup (CPU bound)
	log.Infof("Cleaning up overlapping blocks, initial count: %v", len(dest))
	removeOldData(&dest)
	log.Infof("Optimized count: %v", len(dest))

	// 2. Initialize the Canvas (RGBA buffer)
	// We create a fresh RGBA image for the photo.
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))

	// OPTIONAL: Fill background with white or a default texture
	// If you want a background, you can fastBlit a 'white_concrete' texture in a loop here.
	// For now, it defaults to transparent/black.

	// 3. Render Phase: The "Fast-Blit" loop
	start := time.Now()
	for _, block := range dest {
		tex, ok := getRawTexture(block.BlockTexture)
		if !ok {
			// Subtly log missing textures without spamming
			continue
		}

		// Map block coordinates to pixel coordinates
		posX := int(block.X) * textureSize
		posY := int(block.Y) * textureSize

		fastBlit(canvas, tex, posX, posY)
	}
	log.Successf("Canvas rendered in %v", time.Since(start))

	// 4. Encode to File
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("could not create file: %w", err)
	}
	defer f.Close()

	// Using the standard library PNG encoder for the final output
	if err = png.Encode(f, canvas); err != nil {
		return fmt.Errorf("png encoding failed: %w", err)
	}

	log.Successf("Photo saved to: %s", filename)
	return nil
}

// verifyVideoFile uses ffprobe to ensure the GPU encoder produced a valid stream
func verifyVideoFile(filename string) {
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
		log.Debugf("ffprobe output: %s", string(output))
		return
	}

	stats := strings.ReplaceAll(string(output), "\n", " | ")
	log.Successf("Video Verified: %s", stats)
}
