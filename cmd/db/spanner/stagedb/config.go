package stagedb

import (
	"context"

	dbinitiator "github.com/cccteam/db-initiator"
	"github.com/go-playground/errors/v5"
	"github.com/sethvargo/go-envconfig"
)

type envConfig struct {
	SpannerProjectID          string `env:"GOOGLE_CLOUD_SPANNER_PROJECT,required"`
	SpannerInstanceID         string `env:"GOOGLE_CLOUD_SPANNER_INSTANCE_ID,required"`
	SpannerSourceDatabaseName string `env:"GOOGLE_CLOUD_SPANNER_DATABASE_NAME,required"`
	SpannerMaxBackupAge       int64  `env:"GOOGLE_CLOUD_SPANNER_DATABASE_MAX_AGE,required"`
}

type config struct {
	spanner *dbinitiator.SpannerBackup
}

func newConfig(ctx context.Context) (*config, error) {
	var envVars envConfig
	if err := envconfig.Process(ctx, &envVars); err != nil {
		return nil, errors.Wrap(err, "envconfig.Process()")
	}
	dbConfig, err := dbinitiator.NewSpannerBackup(
		ctx,
		&dbinitiator.SpannerBackup{
			ProjectID:    envVars.SpannerProjectID,
			InstanceID:   envVars.SpannerInstanceID,
			SourceDb:     envVars.SpannerSourceDatabaseName,
			MaxBackupAge: envVars.SpannerMaxBackupAge,
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "dbinitiator.NewSpannerBackup()")
	}

	return &config{
		spanner: dbConfig,
	}, nil
}
