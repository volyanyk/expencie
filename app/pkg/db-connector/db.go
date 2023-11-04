package db_connector

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/pkg/errors"
	"github.com/uptrace/bun/driver/pgdriver"
	gorm_pg "gorm.io/driver/postgres"
	gorm_lib "gorm.io/gorm"
)

type GormPostgresConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	DBName   string `mapstructure:"dbName"`
	SSLMode  bool   `mapstructure:"sslMode"`
	Password string `mapstructure:"password"`
}

type Gorm struct {
	DB     *gorm_lib.DB
	config *GormPostgresConfig
}

func NewGorm(config *GormPostgresConfig) (*gorm_lib.DB, error) {

	var dataSourceName string

	if config.DBName == "" {
		return nil, errors.New("DBName is required in the config.")
	}

	err := createDB(config)

	if err != nil {
		return nil, err
	}

	dataSourceName = fmt.Sprintf("host=%s port=%d user=%s dbname=%s password=%s",
		config.Host,
		config.Port,
		config.User,
		config.DBName,
		config.Password,
	)

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 10 * time.Second
	maxRetries := 5

	var gormDb *gorm_lib.DB

	err = backoff.Retry(func() error {

		gormDb, err = gorm_lib.Open(gorm_pg.Open(dataSourceName), &gorm_lib.Config{})

		if err != nil {
			return errors.Errorf("failed to connect postgres: %v and connection information: %s", err, dataSourceName)
		}

		return nil

	}, backoff.WithMaxRetries(bo, uint64(maxRetries-1)))

	return gormDb, err
}

func (db *Gorm) Close() {
	d, _ := db.DB.DB()
	_ = d.Close()
}

func createDB(cfg *GormPostgresConfig) error {

	datasource := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		"postgres",
	)

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(datasource)))

	var exists int
	rows, err := sqldb.Query(fmt.Sprintf("SELECT 1 FROM pg_catalog.pg_database WHERE datname='%s'", cfg.DBName))
	if err != nil {
		return err
	}

	if rows.Next() {
		err = rows.Scan(&exists)
		if err != nil {
			return err
		}
	}

	if exists == 1 {
		return nil
	}

	_, err = sqldb.Exec(fmt.Sprintf("CREATE DATABASE %s", cfg.DBName))
	if err != nil {
		return err
	}

	defer sqldb.Close()

	return nil
}
