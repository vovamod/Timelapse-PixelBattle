package main

import (
	"Timelapse-PixelBattle/common"
	"Timelapse-PixelBattle/db"
	"Timelapse-PixelBattle/graphics"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-faster/errors"
	ffmpeg "github.com/u2takey/ffmpeg-go"
	"github.com/vovamod/utils/log"
)

func init() {
	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		_, _ = fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}
	// Ignore ffmpeg debug cmd.Compile()
	ffmpeg.LogCompiledCommand = false
}

func main() {
	width := flag.Int("width", 1080, "Width of canvas")
	height := flag.Int("height", 1920, "Height of canvas")
	iterations := flag.Int("iterations", 16, "Number of action per frame")
	textureSize := flag.Int("texture-size", 16, "Texture size (default 16)")
	filename := flag.String("filename", "", "Filename to save video")
	framerate := flag.Int("framerate", 24, "Framerate (default 24)")
	localMode := flag.String("local-mode", "", "Local mode (specify .sql file to load)")
	photo := flag.Bool("photo", false, "Generate a photo of current timelapse only")
	debug := flag.Bool("debug", false, "Debug mode")

	databaseIp := flag.String("database-ip", "", "IP address of database with port")
	databaseUser := flag.String("database-user", "", "Database user")
	databasePassword := flag.String("database-password", "", "Database password")
	databaseName := flag.String("database-name", "", "Database name")

	flag.Parse()
	if *filename == "" {
		flag.Usage()
		log.Error("--filename is required, set your video output name (include file format)")
		// do NOT show stacktrace in case of 0 flags
		os.Exit(1)
	}
	preFilter(*width, *height, *iterations, *textureSize, *framerate, *filename, *localMode, *databaseIp, *databaseUser, *databasePassword, *databaseName, *debug, *photo)
}

func preFilter(width, height, iterations, textureSize, framerate int, filename, localMode, databaseIp, databaseUser, databasePassword, databaseName string, debug, photo bool) {
	log.Info("Running pre config...")
	timer := time.Now()
	switch localMode {
	case "":
		if err := normalModeSetup(width, height, iterations, textureSize, framerate, filename, databaseIp, databaseUser, databasePassword, databaseName, photo, debug); err != nil {
			log.Fatal("Error during executing normalMode generation:", err)
		}
	default:
		if err := localModeSetup(width, height, iterations, textureSize, framerate, filename, localMode, photo, debug); err != nil {
			log.Fatal("Error during executing localMode generation:", err)
		}
	}
	log.Success("Application finished in %v", time.Since(timer))
}

// TODO: FYI. this mode assumes the db name is default and table PB. why? simple. The plugin that comes along for MC is using table PB
func localModeSetup(width, height, iterations, textureSize, framerate int, filename, localMode string, photo, debug bool) error {
	log.Info("Running local mode")
	content, err := os.ReadFile(localMode)
	if err != nil {
		return errors.New("Error reading file: " + err.Error())
	}
	log.Info(fmt.Sprintf("File %s loaded. Current memory usage: %.2f MB", localMode, float64(len(content))/(1024*1024)))

	text := string(content)
	var values []string
	var blocks []common.VisualData
	log.Info("Loading raw input data into schemas...")
	for _, v := range strings.Split(text, "\n") {
		if v == "" {
			continue
		}
		v = strings.Replace(v, "(", "", -1)
		v = strings.Replace(v, ")", "", -1)
		v = strings.Replace(v, "'", "", -1)
		v = strings.Replace(v, ";", "", -1)
		v = strings.Replace(v, ",", "", -1)
		_, v, _ = strings.Cut(v, "INSERT INTO default.PB timestamp x y c VALUES ")
		values = strings.Split(v, " ")
		timestamp, _ := time.Parse("2006-01-02T15:04:05.000000", values[0])
		x, _ := strconv.ParseInt(values[1], 10, 64)
		y, _ := strconv.ParseInt(values[2], 10, 64)
		blocks = append(blocks, common.VisualData{Time: timestamp, X: x, Y: y, BlockTexture: strings.ToLower(values[3]) + ".png"})
	}
	log.Info("Loaded %d raw data schemas. Loading graphics", len(blocks))
	defer func() {
		log.Info("Purging buffer")
		blocks = nil
	}()
	if photo {
		return graphics.GeneratePhotoLocal(blocks, width, height, textureSize, filename)
	}
	return graphics.EncodeGPU(blocks, width, height, iterations, textureSize, framerate, filename, debug)
}

func normalModeSetup(width, height, iterations, textureSize, framerate int, filename, databaseIp, databaseUser, databasePassword, databaseName string, photo, debug bool) error {
	log.Info("Running normal mode")
	log.Notice("Issues may occur if DB fails. This method is still under development!")
	db.Init(databaseIp, databaseUser, databasePassword, databaseName)
	defer db.Close()
	num, _ := db.GetMaxCount()
	var data []common.VisualData
	log.Info("Current db record count is %d", num)
	log.Info("Loading data in 1000 record batches...")
	for i := 0; i <= num; i += 1000 {
		sub := db.GetData(i)
		data = append(data, *sub...)
		runtime.GC()
		log.Info("Parsed %v out of %v", len(data), num)
		num, _ = db.GetMaxCount() // to keep track of NEW records
	}
	log.Info("Loaded %d raw data schemas. Loading graphics", len(data))
	if photo {
		return graphics.GeneratePhotoLocal(data, width, height, textureSize, filename)
	}
	return graphics.EncodeGPU(data, width, height, iterations, textureSize, framerate, filename, debug)
}
