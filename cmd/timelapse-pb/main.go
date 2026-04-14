package main

import (
	"Timelapse-PixelBattle/internal/db"
	"Timelapse-PixelBattle/internal/graphics"
	"Timelapse-PixelBattle/pkg/entities"
	"time"

	"github.com/alecthomas/kong"
	"github.com/vovamod/utils/log"
)

func main() {
	var cli entities.CLI
	log.RegisterCustom("info", log.ColorBrightGreen, nil)
	ctx := kong.Parse(&cli)

	if cli.Debug {
		log.SetType(log.LoggerDebug)
	}

	err := graphics.LoadTextureAtlas("assets", cli.TextureSize)
	if err != nil {
		log.Fatalf("Could not load textures: %v", err)
	}
	timer := time.Now()

	//  LOAD DB
	data := loadData(cli.PlayerName, cli.DBSource, cli.DBIp, cli.DBUser, cli.DBPassword, cli.DBName, cli.DBTable, cli.Local)

	switch ctx.Command() {
	case "render":

		err = graphics.EncodeGPU(*data, cli.Width, cli.Height, cli.Iterations, cli.TextureSize, cli.Framerate, cli.Render.Output, cli.PlayerName, cli.WithInfo, cli.Debug)
	case "photo":
		err = graphics.GeneratePhotoLocal(data, cli.Width, cli.Height, cli.TextureSize, cli.Photo.Output)
	}

	if err != nil {
		log.Errorf("Application failed: %v", err)
		return
	}

	log.Successf("Application finished in %v", time.Since(timer))
}

func loadData(playername, dbSource, dbIp, dbUser, dbPassword, dbName, dbTable string, local bool) *[]entities.VisualData {
	log.Infof("Retrieving data from database: %s", dbName)
	db.Init(dbSource, dbIp, dbUser, dbPassword, dbName, local)
	num, _ := db.GetMaxCount(dbTable, playername)
	data := make([]entities.VisualData, 0, num)
	var id int64
	var timestamp time.Time
	startTime := time.Now()
	log.Infof("Current db record count is %d", num)
	for i := 0; i <= num; i += 1000 {
		sub := db.GetData(playername, dbTable, id, timestamp)
		if sub == nil || len(*sub) == 0 {
			break
		}
		data = append(data, *sub...)

		lastItem := (*sub)[len(*sub)-1]
		id = lastItem.Id
		timestamp = lastItem.Time

		elapsed := time.Since(startTime).Seconds()
		recordsPerSecond := 0
		if elapsed > 0 {
			recordsPerSecond = int(float64(len(data)) / elapsed)
		}

		log.CustomStreamf("info", "Parsed %v out of %v. Est parse speed: %v/s", len(data), num, recordsPerSecond)
		//num, _ = db.GetMaxCount(dbTable, playername) // to keep track of NEW records // 09.04.2026 - retired. not recommended to be runed in real environment
	}
	db.Close()
	return &data
}
