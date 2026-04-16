package db

import (
	"Timelapse-PixelBattle/pkg/entities"
	"context"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/vovamod/utils/log"
)

func retrieveFromCh(query string, args []any) *[]entities.VisualData {
	rowsCh, err := clientCH.Query(context.Background(), query, args...)
	if err != nil {
		log.Error("An error occurred during data retrieval. Error: " + err.Error())
		return new([]entities.VisualData)
	}
	defer func(rows driver.Rows) {
		err = rows.Close()
		if err != nil {
			log.Warn("An error occurred while closing rows, ignoring. Error: " + err.Error())
		}
	}(rowsCh)

	if rowsCh == nil {
		log.Error("Exception! No rows in DB or client failed?")
		return new([]entities.VisualData)
	}

	var preparedData []entities.VisualData
	for rowsCh.Next() {
		var singleData entities.VisualData
		if err = rowsCh.Scan(&singleData.Id, &singleData.Time, &singleData.X, &singleData.Y, &singleData.BlockTexture, &singleData.Owner); err != nil {
			log.Warn("An error occurred while reading row, ignoring. Error: " + err.Error())
		}
		if singleData.BlockTexture == "" || &singleData.X == nil || &singleData.Y == nil {
			continue
		}

		singleData.BlockTexture = strings.ToLower(singleData.BlockTexture) + ".png"
		preparedData = append(preparedData, singleData)
	}

	if err = rowsCh.Err(); err != nil {
		log.Warn("An error occurred while reading some rows, ignoring. Error: " + err.Error())
	}
	return &preparedData
}
