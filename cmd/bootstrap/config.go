package bootstrap

import (
	"context"
	"log"

	initiator "github.com/cccteam/db-initiator"
	"github.com/go-playground/errors/v5"
	"github.com/sethvargo/go-envconfig"
)

type envConfig struct {
	SpannerProjectID       string
	SpannerInstanceID      string
	SpannerDatabaseName    string
	SchemaMigrationDirPath string
}

type config struct {
	migrateClient *initiator.SpannerMigrationService
}

func newConfig(ctx context.Context) (*config, error) {
	var envVars envConfig
	if err := envconfig.Process(ctx, &envVars); err != nil {
		return nil, errors.Wrap(err, "envconfig.Process()")
	}

	db, err := initiator.ConnectToSpanner(ctx, envVars.SpannerProjectID, envVars.SpannerInstanceID, envVars.SpannerDatabaseName)
	if err != nil {
		return nil, errors.Wrapf(err, "initiator.ConnectToSpanner()")
	}

	return &config{
		migrateClient: db,
	}, nil
}

func (c *config) close() {
	if err := c.migrateClient.Close(); err != nil {
		log.Println(err)
	}
}
