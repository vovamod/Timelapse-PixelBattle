package common

import "time"

// Example of data in clickhouse DB:
// 2025-03-27T23:27:48.858300+03:00,382,149,RED_CONCRETE
// go to github.com/vovamod/Timelapse-PixelBattle-Plugin
type VisualData struct {
	Time         time.Time `json:"timestamp"`
	X            int64     `json:"x"`
	Y            int64     `json:"y"`
	BlockTexture string    `json:"c"`
}
