package postgrescontainer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpg "github.com/volyanyk/expencie/app/pkg/db-connector"
	gormlib "gorm.io/gorm"
)

type PgContainerOptions struct {
	Database  string
	Host      string
	Port      nat.Port
	HostPort  int
	UserName  string
	Password  string
	ImageName string
	Name      string
	Tag       string
	Timeout   time.Duration
}

func Start(ctx context.Context, t *testing.T) (*gormlib.DB, error) {

	defaultPostgresOptions, err := getDefaultPostgresTestContainers()
	if err != nil {
		return nil, err
	}

	req := getContainerRequest(defaultPostgresOptions)

	postgresContainer, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})

	if err != nil {
		return nil, err
	}

	// Clean up the container after the test is complete
	t.Cleanup(func() {
		if err := postgresContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	})

	var gormDB *gormlib.DB
	var gormConfig *gormpg.GormPostgresConfig

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 10 * time.Second
	maxRetries := 5

	err = backoff.Retry(func() error {

		host, err := postgresContainer.Host(ctx)
		if err != nil {

			return errors.Errorf("failed to get container host: %v", err)
		}

		realPort, err := postgresContainer.MappedPort(ctx, defaultPostgresOptions.Port)

		if err != nil {
			return errors.Errorf("failed to get exposed container port: %v", err)
		}

		containerPort := realPort.Int()

		gormConfig = &gormpg.GormPostgresConfig{
			Port:     containerPort,
			Host:     host,
			DBName:   defaultPostgresOptions.Database,
			User:     defaultPostgresOptions.UserName,
			Password: defaultPostgresOptions.Password,
			SSLMode:  false,
		}
		gormDB, err = gormpg.NewGorm(gormConfig)
		if err != nil {
			return err
		}
		return nil
	}, backoff.WithMaxRetries(bo, uint64(maxRetries-1)))

	if err != nil {
		return nil, errors.Errorf("failed to create connection for postgres after retries: %v", err)
	}

	return gormDB, nil
}

func getContainerRequest(opts *PgContainerOptions) testcontainers.ContainerRequest {

	containerReq := testcontainers.ContainerRequest{
		Image:        fmt.Sprintf("%s:%s", opts.ImageName, opts.Tag),
		ExposedPorts: []string{"5432/tcp"},
		WaitingFor:   wait.ForListeningPort("5432/tcp"),
		Env: map[string]string{
			"POSTGRES_DB":       opts.Database,
			"POSTGRES_PASSWORD": opts.Password,
			"POSTGRES_USER":     opts.UserName,
		},
	}

	return containerReq
}

func getDefaultPostgresTestContainers() (*PgContainerOptions, error) {
	port, err := nat.NewPort("", "5432")
	if err != nil {
		return nil, fmt.Errorf("failed to build port: %v", err)
	}

	return &PgContainerOptions{
		Database:  "test_db",
		Port:      port,
		Host:      "localhost",
		UserName:  "testcontainers",
		Password:  "testcontainers",
		Tag:       "latest",
		ImageName: "postgres",
		Name:      "postgresql-testcontainer",
		Timeout:   5 * time.Minute,
	}, nil
}
