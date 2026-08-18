package common

import (
	"Timelapse-PixelBattle/pkg/entities"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/vovamod/utils/log"
)

func GetAvailableGPUs() []entities.GPU {
	switch runtime.GOOS {
	case "windows":
		return getWindowsGPUs()
	case "linux":
		return getLinuxGPUs()
	case "darwin":
		return getDarwinGPUs()
	default:
		log.Warn("GPU detection not supported on this OS")
		return []entities.GPU{}
	}
}

// Windows implementation (HM bro)
func getWindowsGPUs() []entities.GPU {
	cmd := exec.Command("powershell", "-Command",
		"Get-CimInstance Win32_VideoController | Select-Object Name, AdapterCompatibility | ConvertTo-Json")

	output, err := cmd.Output()
	if err != nil {
		return []entities.GPU{}
	}

	var gpus []entities.GPU
	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return gpus
	}

	// windows moment.. Bruh :[
	type winGPU struct {
		Name                 string `json:"Name"`
		AdapterCompatibility string `json:"AdapterCompatibility"`
	}
	var results []winGPU
	if raw[0] == '{' {
		var single winGPU
		err = json.Unmarshal([]byte(raw), &single)
		if err != nil {
			log.Warn("Failed to unmarshal JSON from winGPU. Error: " + err.Error())
			return nil
		}
		results = append(results, single)
	} else {
		err = json.Unmarshal([]byte(raw), &results)
		if err != nil {
			log.Warn("Failed to unmarshal JSON from winGPU. Error: " + err.Error())
			return nil
		}
	}

	for _, rg := range results {
		name := strings.ToLower(rg.Name)
		vendor := identifyVendor(name)

		gpus = append(gpus, entities.GPU{
			Name:         rg.Name,
			Vendor:       vendor,
			IsIntegrated: vendor == "intel" || strings.Contains(name, "graphics"),
		})
	}
	return gpus
}

// Linux Implementation
func getLinuxGPUs() []entities.GPU {
	var gpus []entities.GPU
	cmd := exec.Command("lspci", "-nnk")
	output, _ := cmd.Output()
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if !strings.Contains(line, "VGA") && !strings.Contains(line, "3D controller") {
			continue
		}

		fields := strings.SplitN(line, " ", 2)
		if len(fields) != 2 {
			continue
		}
		pciAddress, name := fields[0], fields[1]
		lowerLine := strings.ToLower(name)
		vendor := identifyVendor(lowerLine)

		gpus = append(gpus, entities.GPU{
			Name:         name,
			Vendor:       vendor,
			IsIntegrated: vendor == "intel" || (vendor == "amd" && strings.Contains(lowerLine, "integrated")),
			PCIAddress:   pciAddress,
		})
	}
	return gpus
}

func ResolveVaapiRenderNode(pciAddress string) string {
	if pciAddress == "" {
		return ""
	}
	entries, err := os.ReadDir("/sys/class/drm")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "renderD") {
			continue
		}
		target, err := os.Readlink("/sys/class/drm/" + e.Name() + "/device")
		if err != nil {
			continue
		}
		if strings.HasSuffix(target, pciAddress) {
			return "/dev/dri/" + e.Name()
		}
	}
	return ""
}

func getDarwinGPUs() []entities.GPU {
	cmd := exec.Command("system_profiler", "SPDisplaysDataType")
	output, err := cmd.Output()
	if err != nil {
		return []entities.GPU{}
	}

	var gpus []entities.GPU
	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Chipset Model:") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "Chipset Model:"))
		vendor := identifyVendor(name)

		isIntegrated := vendor == "apple"
		for j := i + 1; j < len(lines) && j < i+6; j++ {
			busLine := strings.TrimSpace(lines[j])
			if strings.HasPrefix(busLine, "Chipset Model:") {
				break
			}
			if strings.HasPrefix(busLine, "Bus:") {
				isIntegrated = strings.Contains(strings.ToLower(busLine), "built-in")
				break
			}
		}

		gpus = append(gpus, entities.GPU{
			Name:         name,
			Vendor:       vendor,
			IsIntegrated: isIntegrated,
		})
	}
	return gpus
}

func identifyVendor(input string) string {
	input = strings.ToLower(input)
	switch {
	case strings.Contains(input, "apple"):
		return "apple"
	case strings.Contains(input, "nvidia"):
		return "nvidia"
	case strings.Contains(input, "amd") || strings.Contains(input, "ati") || strings.Contains(input, "radeon"):
		return "amd"
	case strings.Contains(input, "intel"):
		return "intel"
	default:
		return "unknown"
	}
}
