package main

import (
	"Timelapse-PixelBattle/db"
	"Timelapse-PixelBattle/entities"
	"Timelapse-PixelBattle/graphics"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vovamod/utils/log"
)

func init() {
	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		_, _ = fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}
	log.RegisterCustom("info", log.ColorBrightGreen, nil)
}

func main() {
	width := flag.Int("width", 1080, "Width of canvas")
	height := flag.Int("height", 1920, "Height of canvas")
	iterations := flag.Int("iterations", 16, "Number of action per frame")
	textureSize := flag.Int("texture-size", 16, "Texture size (default 16)")
	filename := flag.String("filename", "", "Filename to save video")
	framerate := flag.Int("framerate", 24, "Framerate (default 24)")
	photo := flag.Bool("photo", false, "Generate a photo of current timelapse only")
	local := flag.Bool("localDB", false, "Use local DB")
	debug := flag.Bool("debug", false, "Debug mode")

	dbSource := flag.String("db-source", "", "db location (file .db)")
	dbIp := flag.String("db-ip", "", "IP address of db with port")
	dbUser := flag.String("db-user", "", "db user")
	dbPassword := flag.String("db-password", "", "db password")
	dbName := flag.String("db-name", "", "db name")
	dbTable := flag.String("db-table", "", "db table")
	flag.Parse()

	err := graphics.LoadTextureAtlas("assets")
	if err != nil {
		log.Fatalf("Could not load textures: %v", err)
	}
	timer := time.Now()
	err = setup(*width, *height, *iterations, *textureSize, *framerate, *filename, *dbSource, *dbIp, *dbUser, *dbPassword, *dbName, *dbTable, *local, *debug, *photo)
	if err != nil {
		log.Errorf("Application failed: %v", err)
		os.Exit(1)
	}
	log.Successf("Application finished in %v", time.Since(timer))

}

func setup(width, height, iterations, textureSize, framerate int, filename, dbSource, dbIp, dbUser, dbPassword, dbName, dbTable string, photo, local, debug bool) error {
	log.Info("Running pre config...")
	log.Notice("Issues may occur if DB fails. This method is still under development!")
	db.Init(dbSource, dbIp, dbUser, dbPassword, dbName, local)
	defer db.Close()
	num, _ := db.GetMaxCount(dbTable)
	var data []entities.VisualData
	log.Infof("Current db record count is %d", num)
	log.Info("Loading data in 1000 record batches...")
	for i := 0; i <= num; i += 1000 {
		sub := db.GetData(dbTable, i)
		data = append(data, *sub...)
		log.CustomStreamf("info", "Parsed %v out of %v", len(data), num)
		num, _ = db.GetMaxCount(dbTable) // to keep track of NEW records
	}
	log.Infof("Loaded %d raw data schemas. Loading graphics", len(data))
	if photo {
		return graphics.GeneratePhotoLocal(data, width, height, textureSize, filename)
	}
	return graphics.EncodeGPU(data, width, height, iterations, textureSize, framerate, filename, true, debug)
}
