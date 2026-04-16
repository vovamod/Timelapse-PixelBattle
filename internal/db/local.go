package db

import (
	"Timelapse-PixelBattle/pkg/entities"
	"database/sql"
	"strings"

	"github.com/vovamod/utils/log"
)

func retrieveFromSqlite(query string, args []any) *[]entities.VisualData {
	rowsL, err := clientLocal.Query(query, args...)
	if err != nil {
		log.Error("An error occurred during data retrieval. Error: " + err.Error())
		return new([]entities.VisualData)
	}
	defer func(rows *sql.Rows) {
		err = rows.Close()
		if err != nil {
			log.Warn("An error occurred while closing rows, ignoring. Error: " + err.Error())
		}
	}(rowsL)

	if rowsL == nil {
		log.Error("Exception! No rows in DB or client failed?")
		return new([]entities.VisualData)
	}

	var preparedData []entities.VisualData
	for rowsL.Next() {
		var singleData entities.VisualData
		if err = rowsL.Scan(&singleData.Id, &singleData.Time, &singleData.X, &singleData.Y, &singleData.BlockTexture, &singleData.Owner); err != nil {
			log.Warn("An error occurred while reading row, ignoring. Error: " + err.Error())
		}
		if singleData.BlockTexture == "" || &singleData.X == nil || &singleData.Y == nil {
			continue
		}

		singleData.BlockTexture = strings.ToLower(singleData.BlockTexture) + ".png"
		preparedData = append(preparedData, singleData)
	}

	// Check for any errors encountered during iteration
	if err = rowsL.Err(); err != nil {
		log.Warn("An error occurred while reading some rows, ignoring. Error: " + err.Error())
	}

	return &preparedData
}
