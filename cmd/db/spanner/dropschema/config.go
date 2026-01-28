package dropschema

import (
	"context"

	dbinitiator "github.com/cccteam/db-initiator"
	"github.com/go-playground/errors/v5"
	"github.com/sethvargo/go-envconfig"
	"google.golang.org/api/option"
)

type envConfig struct {
	SpannerProjectID    string `env:"GOOGLE_CLOUD_SPANNER_PROJECT"`
	SpannerInstanceID   string `env:"GOOGLE_CLOUD_SPANNER_INSTANCE_ID"`
	SpannerDatabaseName string `env:"GOOGLE_CLOUD_SPANNER_DATABASE_NAME"`
}

type config struct {
	migrateClient *dbinitiator.SpannerMigrator
}

func newConfig(ctx context.Context) (*config, error) {
	var envVars envConfig
	if err := envconfig.Process(ctx, &envVars); err != nil {
		return nil, errors.Wrap(err, "envconfig.Process()")
	}

	db, err := dbinitiator.NewSpannerMigrator(ctx, envVars.SpannerProjectID, envVars.SpannerInstanceID, envVars.SpannerDatabaseName, option.WithTelemetryDisabled())
	if err != nil {
		return nil, errors.Wrapf(err, "dbinitiator.NewSpannerMigrator()")
	}

	return &config{
		migrateClient: db,
	}, nil
}

func (c *config) close() {
	c.migrateClient.Close()
}
