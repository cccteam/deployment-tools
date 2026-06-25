package stagedb

import (
	"context"
	"fmt"

	dbinitiator "github.com/cccteam/db-initiator"
	"github.com/go-playground/errors/v5"
	"github.com/sethvargo/go-envconfig"
)

type envConfig struct {
	SpannerProjectID          string `env:"GOOGLE_CLOUD_SPANNER_PROJECT"`
	SpannerInstanceID         string `env:"GOOGLE_CLOUD_SPANNER_INSTANCE_ID"`
	SpannerSourceDatabaseName string `env:"GOOGLE_CLOUD_SPANNER_DATABASE_NAME"`
	SpannerTargetDatabaseName string
}

type config struct {
	spanner *dbinitiator.SpannerBackup
}

func newConfig(ctx context.Context) (*config, error) {
	var envVars envConfig
	if err := envconfig.Process(ctx, &envVars); err != nil {
		return nil, errors.Wrap(err, "envconfig.Process()")
	}
	db, err := dbinitiator.NewSpannerBackup(
		ctx,
		envVars.SpannerProjectID,
		envVars.SpannerInstanceID,
		envVars.SpannerSourceDatabaseName,
		envVars.SpannerTargetDatabaseName,
	)
	if err != nil {
		return nil, errors.Wrap(err, "spannermigrate.Connect()")
	}

	fmt.Println(db)

	return &config{
		spanner: db,
	}, nil
}
