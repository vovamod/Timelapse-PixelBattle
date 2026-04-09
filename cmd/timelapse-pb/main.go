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

	err := graphics.LoadTextureAtlas("assets")
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
	log.Infof("Current db record count is %d", num)
	timestamp := int64(0)
	for i := 0; i <= num; i += 100 {
		sub := db.GetData(playername, dbTable, timestamp)
		if sub == nil || len(*sub) == 0 {
			break
		}
		data = append(data, *sub...)
		timestamp = data[len(data)-1].Time.Unix() + 1
		log.CustomStreamf("info", "Parsed %v out of %v", len(data), num)
		//num, _ = db.GetMaxCount(dbTable, playername) // to keep track of NEW records // 09.04.2026 - retired. not recommended to be runed in real environment
	}
	db.Close()
	return &data
}
