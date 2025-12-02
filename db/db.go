package db

import (
	"Timelapse-PixelBattle/common"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strings"

	"github.com/vovamod/utils/log"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

var (
	client driver.Conn
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
			Debugf: func(format string, v ...interface{}) {
				log.Info(format, v)
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
			log.Info("Exception [%d] %s \n%s\n", exception.Code, exception.Message, exception.StackTrace)
		}
		return nil, err
	}
	return conn, nil
}

func Init(databaseIp, databaseUser, databasePassword, databaseName string) {
	conn, err := ClickHouseConn(databaseIp, databaseUser, databasePassword, databaseName)
	if err != nil {
		log.Info(err.Error())
		panic(err)
	}
	client = conn
}

func Close() {
	err := client.Close()
	if err != nil {
		log.Error("Error closing clickhouse client: %s", err.Error())
	}
}

func GetData(offset int) *[]common.VisualData {
	var off []common.VisualData
	query := `SELECT timestamp, x, y, c FROM PB ORDER BY timestamp LIMIT 1000 OFFSET ?`

	rows, err := client.Query(context.Background(), query, offset*1000)
	if err != nil {
		log.Info(err.Error())
		return &off
	}
	defer func(rows driver.Rows) {
		err = rows.Close()
		if err != nil {
			log.Info(err.Error())
		}
	}(rows)

	// Prepare a slice to hold the results
	var data []common.VisualData
	// Loop over the rows and scan them into the VisualData struct. (at least not direct []byte impl like previous time)
	for rows.Next() {
		var vd common.VisualData
		if err = rows.Scan(&vd.Time, &vd.X, &vd.Y, &vd.BlockTexture); err != nil {
			log.Info(err.Error())
			return &off
		}
		vd.BlockTexture = strings.ToLower(vd.BlockTexture) + ".png"
		data = append(data, vd)
	}

	// Check for any errors encountered during iteration
	if err = rows.Err(); err != nil {
		log.Info(err.Error())
		return &off
	}

	return &data
}

func GetMaxCount() (int, error) {
	query := `SELECT COUNT(*) FROM PB`
	var totalRecords uint64
	if err := client.QueryRow(context.Background(), query).Scan(&totalRecords); err != nil {
		return 0, err
	}

	return int(totalRecords), nil
}
