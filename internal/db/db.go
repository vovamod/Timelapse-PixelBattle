package db

import (
	"Timelapse-PixelBattle/pkg/entities"
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
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

func GetData(playername string, table string, lastTimestamp int64) *[]entities.VisualData {
	var singleData entities.VisualData
	var preparedData []entities.VisualData
	var rowsCh driver.Rows
	var rowsL *sql.Rows
	var err error
	var queryS strings.Builder

	queryS.WriteString(fmt.Sprintf("SELECT timestamp, x, y, c, owner FROM %s WHERE 1=1", table))
	var args []interface{}
	if playername != "" {
		queryS.WriteString(" AND owner = ?")
		args = append(args, playername)
	}

	if lastTimestamp > 0 {
		queryS.WriteString(" AND timestamp > ?")
		args = append(args, lastTimestamp)
	}
	queryS.WriteString(" ORDER BY timestamp ASC LIMIT 100")
	query := queryS.String()

	if local != true {
		rowsCh, err = clientCH.Query(context.Background(), query, args...)
		if err != nil {
			log.Error(err.Error())
			return new([]entities.VisualData)
		}
		defer func(rows driver.Rows) {
			err = rows.Close()
			if err != nil {
				log.Info(err.Error())
			}
		}(rowsCh)
	} else {
		rowsL, err = clientLocal.Query(query, args...)
		if err != nil {
			log.Error(err.Error())
			return new([]entities.VisualData)
		}
		defer func(rows *sql.Rows) {
			err = rows.Close()
			if err != nil {
				log.Info(err.Error())
			}
		}(rowsL)
	}

	if !local {
		for rowsCh.Next() {
			if err = rowsCh.Scan(&singleData.Time, &singleData.X, &singleData.Y, &singleData.BlockTexture, &singleData.Owner); err != nil {
				log.Error(err.Error())
				return new([]entities.VisualData)
			}

			if singleData.BlockTexture == "" {
				continue
			}

			singleData.BlockTexture = strings.ToLower(singleData.BlockTexture) + ".png"
			preparedData = append(preparedData, singleData)
			singleData = entities.VisualData{} // clean this mf
		}

		if err = rowsCh.Err(); err != nil {
			log.Info(err.Error())
		}
	} else {
		if rowsL == nil {
			log.Error("Exception! No rows in DB or client failed?")
			return new([]entities.VisualData)
		}
		for rowsL.Next() {
			if err = rowsL.Scan(&singleData.Time, &singleData.X, &singleData.Y, &singleData.BlockTexture, &singleData.Owner); err != nil {
				log.Error(err.Error())
				return new([]entities.VisualData)
			}

			if singleData.BlockTexture == "" {
				continue
			}

			singleData.BlockTexture = strings.ToLower(singleData.BlockTexture) + ".png"
			preparedData = append(preparedData, singleData)
			singleData = entities.VisualData{} // clean this mf
		}

		// Check for any errors encountered during iteration
		if err = rowsL.Err(); err != nil {
			log.Info(err.Error())
		}
	}

	return &preparedData
}

func GetMaxCount(table string, playername string) (int, error) {
	var totalRecords uint64
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)
	if playername != "" {
		query = fmt.Sprintf("%s WHERE owner = '%s'", query, playername)
	}
	if local != true {
		if err := clientCH.QueryRow(context.Background(), query).Scan(&totalRecords); err != nil {
			return 0, err
		}
	} else {
		// 01.04.2026 - If someone will touch this. Know, I fucking hate sqlite with all my soul, I WISH TO BURN THIS SHIT BECAUSE I CANNOT USE ? as table name... ONLY F*CKING VALUES allowed.
		if err := clientLocal.QueryRow(query).Scan(&totalRecords); err != nil {
			log.Errorf("Error getting max count: %s", err.Error())
			return 0, err
		}
	}
	return int(totalRecords), nil
}
