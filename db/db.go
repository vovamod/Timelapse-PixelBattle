package db

import (
	"Timelapse-PixelBattle/entities"
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	_ "github.com/go-sql-driver/mysql"
	"github.com/vovamod/utils/log"
	_ "modernc.org/sqlite"
)

var (
	clientCH    driver.Conn
	clientLocal *sql.DB
	local       = false
)

func ClickHouseConn(databaseIp, databaseUser, databasePassword, databaseName string) (driver.Conn, error) {
	var (
		dialCount = 0
		ctx       = context.Background()
		conn, err = clickhouse.Open(&clickhouse.Options{
			Addr: []string{databaseIp},
			Auth: clickhouse.Auth{
				Database: databaseName,
				Username: databaseUser,
				Password: databasePassword,
			},
			ClientInfo: clickhouse.ClientInfo{
				Products: []struct {
					Name    string
					Version string
				}{
					{Name: "Timelapse-machine", Version: "0.1.0"},
				},
			},
			DialContext: func(ctx context.Context, addr string) (net.Conn, error) {
				dialCount++
				var d net.Dialer
				return d.DialContext(ctx, "tcp", addr)
			},
			TLS: &tls.Config{
				InsecureSkipVerify: true,
			},
			Settings: clickhouse.Settings{
				"max_execution_time": 60,
			},
			Compression: &clickhouse.Compression{
				Method: clickhouse.CompressionLZ4,
			},
		})
	)

	if err != nil {
		return nil, err
	}

	if err = conn.Ping(ctx); err != nil {
		var exception *clickhouse.Exception
		if errors.As(err, &exception) {
			log.Errorf("Exception [%d] %s \n%s\n", exception.Code, exception.Message, exception.StackTrace)
		}
		return nil, err
	}
	return conn, nil
}

func Init(databaseSource, databaseIp, databaseUser, databasePassword, databaseName string, localOnly bool) {
	local = localOnly
	if localOnly == true {
		conn, err := sql.Open("sqlite", databaseSource)
		if err != nil {
			log.Fatal(err.Error())
		}
		clientLocal = conn
	} else {
		conn, err := ClickHouseConn(databaseIp, databaseUser, databasePassword, databaseName)
		if err != nil {
			log.Fatal(err.Error())
		}
		clientCH = conn
	}
}

func Close() {
	if local != true {
		err := clientCH.Close()
		if err != nil {
			log.Errorf("Error closing clickhouse client: %s", err.Error())
		}
	} else {
		err := clientLocal.Close()
		if err != nil {
			log.Errorf("Error closing sql client: %s", err.Error())
		}
	}
}

func GetData(table string, offset int) *[]entities.VisualData {
	var singleData entities.VisualData
	var preparedData []entities.VisualData
	var rowsCh driver.Rows
	var rowsL *sql.Rows
	var err error

	if local != true {
		rowsCh, err = clientCH.Query(context.Background(), fmt.Sprintf(`SELECT timestamp, x, y, c, owner FROM %s ORDER BY timestamp LIMIT 1000 OFFSET ?`, table), offset*1000)
		if err != nil {
			log.Info(err.Error())
			return new([]entities.VisualData)
		}
		defer func(rows driver.Rows) {
			err = rows.Close()
			if err != nil {
				log.Info(err.Error())
			}
		}(rowsCh)
	} else {
		rowsL, err = clientLocal.Query(fmt.Sprintf(`SELECT timestamp, x, y, c, owner FROM %s ORDER BY timestamp LIMIT 1000 OFFSET ?`, table), offset)
		if err != nil {
			log.Info(err.Error())
			return new([]entities.VisualData)
		}
		defer func(rows *sql.Rows) {
			err = rows.Close()
			if err != nil {
				log.Info(err.Error())
			}
		}(rowsL)
	}

	if local != true {
		for rowsCh.Next() {
			if err = rowsCh.Scan(&singleData.Time, &singleData.X, &singleData.Y, &singleData.BlockTexture, &singleData.Owner); err != nil {
				log.Info(err.Error())
				return new([]entities.VisualData)
			}
			singleData.BlockTexture = strings.ToLower(singleData.BlockTexture) + ".png"
			preparedData = append(preparedData, singleData)
			singleData = entities.VisualData{} // clean this mf
		}

		if err = rowsCh.Err(); err != nil {
			log.Info(err.Error())
			return new([]entities.VisualData)
		}
	} else {
		if rowsL == nil {
			log.Error("Exception! No rows in DB or client failed?")
			return new([]entities.VisualData)
		}
		for rowsL.Next() {
			if err = rowsL.Scan(&singleData.Time, &singleData.X, &singleData.Y, &singleData.BlockTexture, &singleData.Owner); err != nil {
				log.Info(err.Error())
				return new([]entities.VisualData)
			}
			singleData.BlockTexture = strings.ToLower(singleData.BlockTexture) + ".png"
			preparedData = append(preparedData, singleData)
			singleData = entities.VisualData{} // clean this mf
		}

		// Check for any errors encountered during iteration
		if err = rowsL.Err(); err != nil {
			log.Info(err.Error())
			return new([]entities.VisualData)
		}
	}

	return &preparedData
}

func GetMaxCount(table string) (int, error) {
	var totalRecords uint64
	if local != true {
		if err := clientCH.QueryRow(context.Background(), `SELECT COUNT(*) FROM ?`, table).Scan(&totalRecords); err != nil {
			return 0, err
		}
	} else {
		// 01.04.2026 - If someone will touch this. Know, I fucking hate sqlite with all my soul, I WISH TO BURN THIS SHIT BECAUSE I CANNOT USE ? as table name... ONLY F*CKING VALUES allowed.
		if err := clientLocal.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&totalRecords); err != nil {
			log.Errorf("Error getting max count: %s", err.Error())
			return 0, err
		}
	}
	return int(totalRecords), nil
}
