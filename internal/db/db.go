package db

import (
	"Timelapse-PixelBattle/pkg/entities"
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"net"

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

const (
	TableSelect = `SELECT id, timestamp, x, y, c, owner FROM `
	TableCount  = `SELECT COUNT(*) FROM `
)

func ClickHouseConn(databaseIp, databaseUser, databasePassword, databaseName string, databaseTLSEnabled bool) (driver.Conn, error) {
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
					{Name: "timelapse-pixelbattle", Version: "2.1.0"},
				},
			},
			DialContext: func(ctx context.Context, addr string) (net.Conn, error) {
				dialCount++
				var d net.Dialer
				return d.DialContext(ctx, "tcp", addr)
			},
			TLS: &tls.Config{
				InsecureSkipVerify: !databaseTLSEnabled,
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

func Init(databaseSource, databaseIp, databaseUser, databasePassword, databaseName string, databaseTLSEnabled, localOnly bool) {
	local = localOnly
	if localOnly == true {
		conn, err := sql.Open("sqlite", databaseSource)
		if err != nil {
			log.Fatal(err.Error())
		}
		clientLocal = conn
	} else {
		conn, err := ClickHouseConn(databaseIp, databaseUser, databasePassword, databaseName, databaseTLSEnabled)
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

func GetData(playername string, table string, id int64) *[]entities.VisualData {
	query, args := buildQuery(playername, table, id)

	// Split logic for easier reading
	if local {
		return retrieveFromSqlite(query, args)
	}
	return retrieveFromCh(query, args)
}

func GetMaxCount(table string, playername string) (int, error) {
	var totalRecords uint64
	query := TableCount + table
	if playername != "" {
		query = fmt.Sprintf("%s WHERE owner = '%s'", query, playername)
	}

	if local != true {
		if err := clientCH.QueryRow(context.Background(), query).Scan(&totalRecords); err != nil {
			log.Errorf("Error getting max count: %s", err.Error())
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

func buildQuery(playername string, table string, id int64) (string, []any) {
	base := TableSelect + table

	switch {
	case playername != "" && id != 0:
		return base + " WHERE owner = ? AND id > ? ORDER BY id LIMIT 10000",
			[]any{playername, id}

	case playername != "":
		return base + " WHERE owner = ? ORDER BY id LIMIT 10000",
			[]any{playername}

	case id != 0:
		return base + " WHERE id > ? ORDER BY id LIMIT 10000",
			[]any{id}

	default:
		return base + " ORDER BY id LIMIT 10000", nil
	}
}
