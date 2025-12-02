package graphics

import (
	"Timelapse-PixelBattle/common"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/vovamod/utils/log"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

var (
	imgTmp      *image.Image
	frameBuffer []byte
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

	pr, pw := io.Pipe()

	ffmpegArgs := ffmpeg.KwArgs{
		"f":       "rawvideo",
		"pix_fmt": "rgb24",
		"s":       fmt.Sprintf("%dx%d", width, height),
		"r":       fmt.Sprintf("%d", framerate),
	}

	outputArgs := getEncoderArgs(encoder, encoderName, gpuType, width, height, needsScaling)

	var ffErr error
	var wg sync.WaitGroup
	ffmpegDone := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		if needsScaling {
			log.Info("FFmpeg process starting with GPU encoding and automatic scaling...")
		} else {
			log.Info("FFmpeg process starting with GPU encoding...")
		}

		cmd := ffmpeg.Input("pipe:0", ffmpegArgs).
			Output(filename, outputArgs).
			OverWriteOutput().
			WithInput(pr)

		if debug {
			ffErr = cmd.WithOutput(os.Stdout, os.Stderr).Run()
		} else {
			ffErr = cmd.Run()
		}

		if ffErr != nil {
			log.Error(fmt.Sprintf("FFmpeg error: %v", ffErr))
		} else {
			log.Info("FFmpeg process completed successfully")
		}
		close(ffmpegDone)
	}()

	time.Sleep(2 * time.Second)

	// Writer: produce frames and write to ffmpeg stdin (pw)
	writeErr := (func() error {
		defer func() {
			_ = pw.Close()
		}()

		batchSize := iterations
		frameIndex := 0
		totalFrames := (len(dest) + batchSize - 1) / batchSize

		for i := 0; i < len(dest); i += batchSize {
			select {
			case <-ffmpegDone:
				if ffErr != nil {
					return fmt.Errorf("ffmpeg exited before frame %d: %w", frameIndex, ffErr)
				}
				return fmt.Errorf("ffmpeg exited before frame %d", frameIndex)
			default:
			}

			timer := time.Now()
			end := i + batchSize
			if end > len(dest) {
				end = len(dest)
			}
			batch := dest[i:end]

			img := frameCreate(&batch, imgTmp, width, height, textureSize, &frameBuffer)
			if img == nil {
				return fmt.Errorf("render returned nil image at frame %d", frameIndex)
			}

			imgTmp = &img

			// Use the pre-filled frameBuffer from frameCreateBuffered
			log.Debug("Buffer check: %v", len(frameBuffer))
			n, err := pw.Write(frameBuffer)
			if err != nil {
				return fmt.Errorf("writing to ffmpeg pipe failed after %d bytes at frame %d: %w", n, frameIndex, err)
			}

			frameIndex++
			progress := float64(frameIndex) / float64(totalFrames) * 100
			log.Info("Rendered frame %d/%d (%.1f%%) in %v",
				frameIndex, totalFrames, progress, time.Since(timer))
		}

		log.Info("All frames written to pipe successfully")
		return nil
	})()

	wg.Wait()

	if ffErr != nil {
		if writeErr != nil {
			return fmt.Errorf("ffmpeg error: %w; write error: %v", ffErr, writeErr)
		}
		return fmt.Errorf("ffmpeg error: %w", ffErr)
	}
	if writeErr != nil {
		return fmt.Errorf("write error: %w", writeErr)
	}

	log.Info("Video rendering completed successfully")
	verifyVideoFile(filename)

	return nil
}

func GeneratePhotoLocal(dest []common.VisualData, width, height, textureSize int, filename string) error {
	log.Info(fmt.Sprintf("Current configuration:\n  - Width: %v\n  - Height: %v\n  - TextureSize: %v",
		width, height, textureSize))

	log.Info("Removing old blocks with older timestamps, current: %v elems", len(dest))
	removeOldData(&dest)
	log.Info("Current size of data: %v elems", len(dest))
	im := frameCreate(&dest, nil, width, height, textureSize, nil)

	f, err := os.Create(filename)
	if err != nil {
		log.Error("Error while creating file: %v", err.Error())
	}

	if err = png.Encode(f, im); err != nil {
		log.Fatal("Error while encoding png file: %v", err.Error())
	}
	if err = f.Close(); err != nil {
		log.Error("Error while closing file: %v", err.Error())
	}
	return nil
}

// verifyVideoFile checks if the video file is valid
func verifyVideoFile(filename string) {
	// Use ffprobe to check the video
	log.Notice(fmt.Sprintf("Verifying file %s via ffprobe (this may take a bit)", filename))
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-count_frames", "-show_entries", "stream=width,height,nb_frames,codec_name",
		"-of", "csv=p=0", filename)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error("ffprobe failed: %v, output: %s", err, string(output))
	}

	log.Info(fmt.Sprintf("Video verification details: %s", string(output)))
}
