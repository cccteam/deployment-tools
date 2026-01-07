package bootstrap

import (
	"context"

	"github.com/cccteam/deployment-tools/internal/spannermigrate"
	"github.com/go-playground/errors/v5"
	"github.com/sethvargo/go-envconfig"
)

type envConfig struct {
	SpannerProjectID    string `env:"GOOGLE_CLOUD_SPANNER_PROJECT"`
	SpannerInstanceID   string `env:"GOOGLE_CLOUD_SPANNER_INSTANCE_ID"`
	SpannerDatabaseName string `env:"GOOGLE_CLOUD_SPANNER_DATABASE_NAME"`
}

type config struct {
	migrateClient *spannermigrate.Client
}

func newConfig(ctx context.Context) (*config, error) {
	var envVars envConfig
	if err := envconfig.Process(ctx, &envVars); err != nil {
		return nil, errors.Wrap(err, "envconfig.Process()")
	}

	db, err := spannermigrate.Connect(
		ctx,
		envVars.SpannerProjectID,
		envVars.SpannerInstanceID,
		envVars.SpannerDatabaseName,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "spannermigrate.Connect()")
	}

	return &config{
		migrateClient: db,
	}, nil
}

func (c *config) close(ctx context.Context) {
	c.migrateClient.Close(ctx)
}
