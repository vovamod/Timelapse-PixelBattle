package graphics

import (
	"Timelapse-PixelBattle/pkg/common"
	"Timelapse-PixelBattle/pkg/entities"
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/vovamod/utils/log"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// getMaxResolution - maximum size of h264/x264 encode with vaapi
func getMaxResolution(gpuType string) (int, int) {
	switch gpuType {
	case "nvidia":
		return 4096, 4096
	case "amd_discrete":
		return 4096, 4096
	case "amd_integrated":
		return 3840, 2160
	case "intel_integrated":
		return 3840, 2160
	default:
		return 1920, 1080
	}
}

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

// GetGPUEncoder - uses Best GPU encoder IF available for ffmpeg (prime-run can fail.) - Exporting IN ORDER to use in timelapse-cli/timelapse-gui
func GetGPUEncoder(width, height int) entities.GPUSelection {
	allGPUs := common.GetAvailableGPUs()

	if len(allGPUs) == 0 {
		return entities.GPUSelection{Encoder: "libx264", EncoderName: "libx264", GPUType: "cpu"}
	}

	log.Info("Detected GPUs:")
	for i, g := range allGPUs {
		log.Infof("  [%d] %s (%s) - Integrated: %v", i+1, g.Name, g.Vendor, g.IsIntegrated)
	}
	log.Info("Select a GPU by number or name (0 for CPU, Leave empty for auto-selection):")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "0" || strings.EqualFold(input, "cpu") {
		log.Warn("User selected Software Encoder. Proceeding with libx264.")
		return entities.GPUSelection{Encoder: "libx264", EncoderName: "libx264", GPUType: "cpu"}
	}
	var selectedGPU *entities.GPU

	if input != "" {
		if idx, err := strconv.Atoi(input); err == nil && idx > 0 && idx <= len(allGPUs) {
			selectedGPU = &allGPUs[idx-1]
		} else {
			for _, g := range allGPUs {
				if strings.EqualFold(g.Name, input) {
					selectedGPU = &g
					break
				}
			}
		}
	}

	if selectedGPU != nil {
		ent := ResolveEncoderForGPU(*selectedGPU)
		log.Successf("User selected: %s. Using encoder: %s", selectedGPU.Name, ent.Encoder)
		return ent
	}

	log.Info("Proceeding with automated selection...")
	for _, g := range allGPUs {
		if g.Vendor == "nvidia" {
			return ResolveEncoderForGPU(g)
		}
	}

	for _, g := range allGPUs {
		if g.Vendor == "amd" && !g.IsIntegrated {
			return ResolveEncoderForGPU(g)
		}
	}

	if width <= 3840 && height <= 2160 {
		for _, g := range allGPUs {
			if g.IsIntegrated {
				return ResolveEncoderForGPU(g)
			}
		}
	}

	log.Warn("No suitable GPU configuration found, defaulting to CPU")
	return entities.GPUSelection{Encoder: "libx264", EncoderName: "libx264", GPUType: "cpu"}
}

func ResolveEncoderForGPU(g entities.GPU) entities.GPUSelection {
	switch g.Vendor {
	case "nvidia":
		if checkEncoderSupport("hevc_nvenc") {
			return entities.GPUSelection{Encoder: "hevc_nvenc", EncoderName: "nvenc_hevc", GPUType: "nvidia"}
		}
		return entities.GPUSelection{Encoder: "h264_nvenc", EncoderName: "nvenc", GPUType: "nvidia"}

	case "amd":
		if runtime.GOOS == "windows" {
			return entities.GPUSelection{Encoder: "h264_amf", EncoderName: "amf", GPUType: "amd_discrete"}
		}
		return entities.GPUSelection{Encoder: "h264_vaapi", EncoderName: "vaapi", GPUType: "amd_discrete"}

	case "intel":
		if checkEncoderSupport("h264_qsv") {
			return entities.GPUSelection{Encoder: "h264_qsv", EncoderName: "qsv", GPUType: "intel_integrated"}
		}
		return entities.GPUSelection{Encoder: "h264_vaapi", EncoderName: "vaapi", GPUType: "intel_integrated"}

	default:
		return entities.GPUSelection{Encoder: "libx264", EncoderName: "libx264", GPUType: "cpu"}
	}
}

func checkEncoderSupport(codec string) bool {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-encoders")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), codec)
}

func getEncoderArgs(eGPU entities.GPUSelection, width, height int, useScaling bool) ffmpeg.KwArgs {
	baseArgs := ffmpeg.KwArgs{}
	targetWidth, targetHeight := width, height
	if useScaling {
		targetWidth, targetHeight = calculateScaledDimensions(width, height, eGPU.GPUType)
	}
	baseArgs["movflags"] = "faststart"

	switch eGPU.EncoderName {
	case "nvenc", "nvenc_hevc":
		baseArgs["c:v"] = eGPU.Encoder
		baseArgs["preset"] = "p4"
		baseArgs["cq"] = "23"
		baseArgs["rc"] = "constqp"
		baseArgs["color_range"] = "pc"
		baseArgs["colorspace"] = "bt709"
		baseArgs["color_primaries"] = "bt709"
		baseArgs["color_trc"] = "bt709"
		if useScaling {
			baseArgs["vf"] = fmt.Sprintf("hwupload_cuda,scale_cuda=w=%d:h=%d:interp_algo=nearest", targetWidth, targetHeight)
		}
		baseArgs["bf"] = "0"
	case "amf":
		baseArgs["c:v"] = eGPU.Encoder
		baseArgs["quality"] = "quality"
		baseArgs["profile"] = "high"
		if useScaling {
			baseArgs["vf"] = fmt.Sprintf("scale=%d:%d:flags=lanczos,format=yuv420p", targetWidth, targetHeight)
		}
	case "qsv":
		baseArgs["c:v"] = eGPU.Encoder
		baseArgs["preset"] = "quality"
		baseArgs["profile"] = "high"
		if useScaling {
			baseArgs["vf"] = fmt.Sprintf("scale=%d:%d:flags=lanczos,format=yuv420p", targetWidth, targetHeight)
		}
	case "vaapi":
		baseArgs["c:v"] = eGPU.Encoder
		if eGPU.GPUType == "amd_integrated" || eGPU.GPUType == "intel_integrated" {
			baseArgs["vaapi_device"] = "/dev/dri/renderD128"
		}
		if useScaling {
			baseArgs["vf"] = fmt.Sprintf("scale=%d:%d:flags=lanczos", targetWidth, targetHeight)
		}
	default: // libx264 and others
		baseArgs["c:v"] = eGPU.Encoder
		baseArgs["preset"] = "medium"
		baseArgs["crf"] = "23"
	}
	return baseArgs
}
