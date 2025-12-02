package graphics

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/vovamod/utils/log"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// getGPUInfo detects the actual GPU configuration (works on linux only. No Windows support.)
func getGPUInfo() (string, bool) {
	var integrated, discrete string

	// NVIDIA (always discrete)
	cmd := exec.Command("sh", "-c", "lspci | grep -i nvidia")
	if output, err := cmd.Output(); err == nil && len(output) > 0 {
		discrete = "nvidia"
	}

	// AMD
	cmd = exec.Command("sh", "-c", "lspci | grep -i 'amd\\|ati' | grep -i 'vga\\|3d\\|display'")
	if output, err := cmd.Output(); err == nil && len(output) > 0 {
		out := strings.ToLower(string(output))
		if strings.Contains(out, "radeon") {
			discrete = "amd_discrete"
		} else {
			integrated = "amd_integrated"
		}
	}

	// Intel integrated
	cmd = exec.Command("sh", "-c", "lspci | grep -i intel | grep -i 'vga\\|display'")
	if output, err := cmd.Output(); err == nil && len(output) > 0 {
		integrated = "intel_integrated"
	}

	// Priority: integrated first
	if integrated != "" {
		return integrated, true
	}

	// Otherwise return discrete
	if discrete != "" {
		return discrete, false
	}

	return "unknown", false
}

// getMaxResolution - maximum size of h264/x264 encode with vaapi
func getMaxResolution(gpuType string) (int, int) {
	switch gpuType {
	case "nvidia":
		return 8192, 8192 // H264
	case "amd_discrete":
		return 4096, 4096
	case "amd_integrated":
		return 3840, 2160
	case "intel_integrated":
		return 3840, 2160
	default:
		return 1920, 1080 // Safe mode
	}
}

// TODO: some cleanup...
func calculateScaledDimensions(width, height int, gpuType string) (int, int) {
	maxWidth, maxHeight := getMaxResolution(gpuType)

	if width <= maxWidth && height <= maxHeight {
		return width, height
	}

	widthScale := float64(maxWidth) / float64(width)
	heightScale := float64(maxHeight) / float64(height)
	scale := minFloat(widthScale, heightScale)
	scaledWidth := int(float64(width) * scale)
	scaledHeight := int(float64(height) * scale)

	// Standard of YUV420p assumes VALUE%2=true
	if scaledWidth%2 != 0 {
		scaledWidth--
	}
	if scaledHeight%2 != 0 {
		scaledHeight--
	}

	log.Info(fmt.Sprintf("Scaling from %dx%d to %dx%d for %s compatibility",
		width, height, scaledWidth, scaledHeight, gpuType))

	return scaledWidth, scaledHeight
}

// minFloat - helper
func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// getGPUEncoder - uses Best GPU encoder IF available for ffmpeg (prime-run can fail.)
func getGPUEncoder(width, height int) (string, string, string) {
	gpuType, isIntegrated := getGPUInfo()
	log.Info(fmt.Sprintf("Detected GPU: %s (integrated: %v)", gpuType, isIntegrated))

	if isIntegrated && (width > 3840 || height > 2160 || width*height > 8000000) {
		log.Info("High resolution detected with integrated graphics, using CPU encoder for stability")
		return "libx264", "libx264", gpuType
	}

	var encoders []struct {
		name     string
		codec    string
		checkCmd string
	}

	switch gpuType {
	case "nvidia":
		encoders = []struct {
			name     string
			codec    string
			checkCmd string
		}{
			{"nvenc", "h264_nvenc", "ffmpeg -hide_banner -encoders | grep h264_nvenc"},
		}
	case "intel_integrated":
		encoders = []struct {
			name     string
			codec    string
			checkCmd string
		}{
			{"qsv", "h264_qsv", "ffmpeg -hide_banner -encoders | grep h264_qsv"},
			{"vaapi", "h264_vaapi", "ffmpeg -hide_banner -encoders | grep h264_vaapi"},
		}
	default:
		encoders = []struct {
			name     string
			codec    string
			checkCmd string
		}{
			{"nvenc", "h264_nvenc", "ffmpeg -hide_banner -encoders | grep h264_nvenc"},
			{"amf", "h264_amf", "ffmpeg -hide_banner -encoders | grep h264_amf"},
			{"qsv", "h264_qsv", "ffmpeg -hide_banner -encoders | grep h264_qsv"},
			{"vaapi", "h264_vaapi", "ffmpeg -hide_banner -encoders | grep h264_vaapi"},
		}
	}

	for _, encoder := range encoders {
		cmd := exec.Command("sh", "-c", encoder.checkCmd)
		if err := cmd.Run(); err == nil {
			log.Info(fmt.Sprintf("Using GPU encoder: %s (%s) for %s", encoder.name, encoder.codec, gpuType))
			return encoder.codec, encoder.name, gpuType
		}
	}

	log.Info("No compatible GPU encoder found, using CPU encoder")
	return "libx264", "libx264", gpuType
}

func getEncoderArgs(encoder string, encoderName string, gpuType string, width, height int, useScaling bool) ffmpeg.KwArgs {
	baseArgs := ffmpeg.KwArgs{
		"pix_fmt":  "yuv420p",
		"movflags": "faststart",
	}
	targetWidth, targetHeight := width, height
	if useScaling {
		targetWidth, targetHeight = calculateScaledDimensions(width, height, gpuType)
	}

	switch encoderName {
	case "nvenc":
		baseArgs["c:v"] = encoder
		baseArgs["preset"] = "p4"
		baseArgs["cq"] = "23"
		baseArgs["rc"] = "vbr"
		if useScaling {
			baseArgs["vf"] = fmt.Sprintf("scale=%d:%d:flags=lanczos", targetWidth, targetHeight)
		}
	case "amf":
		baseArgs["c:v"] = encoder
		baseArgs["quality"] = "quality"
		baseArgs["profile"] = "high"
		if useScaling {
			baseArgs["vf"] = fmt.Sprintf("scale=%d:%d:flags=lanczos", targetWidth, targetHeight)
		}
	case "qsv":
		baseArgs["c:v"] = encoder
		baseArgs["preset"] = "quality"
		baseArgs["profile"] = "high"
		if useScaling {
			baseArgs["vf"] = fmt.Sprintf("scale=%d:%d:flags=lanczos", targetWidth, targetHeight)
		}
	case "vaapi":
		baseArgs["c:v"] = encoder
		if gpuType == "amd_integrated" || gpuType == "intel_integrated" {
			baseArgs["vaapi_device"] = "/dev/dri/renderD128"
		}
		if useScaling {
			baseArgs["vf"] = fmt.Sprintf("scale=%d:%d:flags=lanczos", targetWidth, targetHeight)
		}
	default: // libx264 and others
		baseArgs["c:v"] = encoder
		baseArgs["preset"] = "medium"
		baseArgs["crf"] = "23"
	}

	return baseArgs
}
