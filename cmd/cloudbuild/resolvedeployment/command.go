package resolvedeployment

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv2"
	"cloud.google.com/go/cloudbuild/apiv2/cloudbuildpb"
	"github.com/go-playground/errors/v5"
	"github.com/google/go-github/v80/github"
	"github.com/sethvargo/go-envconfig"
	"github.com/spf13/cobra"
)

// EnvironmentConfig holds environment variables for GitHub authentication.
type EnvironmentConfig struct {
	ProjectID          string `env:"PROJECT_ID"`
	Location           string `env:"LOCATION"`
	RepoConnectionName string `env:"_REPO_CONNECTION_NAME"`
	RepoName           string `env:"_REPO_NAME"`
}

// Command returns the configured command
func Command(ctx context.Context) *cobra.Command {
	cli := command{}

	return cli.Setup(ctx)
}

type command struct {
	configFile string
	envConfig  *EnvironmentConfig
	config     *Config
	gh         *github.Client
}

// Setup returns the configured cli command
func (c *command) Setup(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve-deployment",
		Short: "Resolve deployment configurations",
		Long:  "Resolve deployment configurations based on the provided context",
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			if err := c.ValidateFlags(); err != nil {
				return err
			}

			if err := c.Run(ctx, cmd); err != nil {
				log.Println(err)
			}

			return nil
		},
	}

	cmd.Flags().
		StringVarP(&c.configFile, "config", "c", "", "Path to JSON config file (optional, defaults to environment variables)")

	return cmd
}

// ValidateFlags validates and processes any input flags
func (c *command) ValidateFlags() error {
	return nil
}

// Run executes the command
func (c *command) Run(ctx context.Context, _ *cobra.Command) error {
	fmt.Println("Resolving deployment configurations...")

	// Load environment config for GitHub authentication
	var envConf EnvironmentConfig
	if err := envconfig.Process(ctx, &envConf); err != nil {
		return errors.Wrap(err, "envconfig.Process(EnvironmentConfig)")
	}
	c.envConfig = &envConf

	// Load resolver config from JSON file or environment variables
	cfg, err := c.loadConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "loadConfig()")
	}
	c.config = cfg

	// Create GitHub client using Cloud Build repository token
	repoManagerClient, err := cloudbuild.NewRepositoryManagerClient(ctx)
	if err != nil {
		return errors.Wrap(err, "cloudbuild.NewRepositoryManagerClient()")
	}
	defer func() {
		if closeErr := repoManagerClient.Close(); closeErr != nil {
			log.Printf("failed to close repoManagerClient: %v", closeErr)
		}
	}()

	req := &cloudbuildpb.FetchReadTokenRequest{
		Repository: fmt.Sprintf(
			"projects/%s/locations/%s/connections/%s/repositories/%s",
			c.envConfig.ProjectID,
			c.envConfig.Location,
			c.envConfig.RepoConnectionName,
			c.envConfig.RepoName,
		),
	}
	resp, err := repoManagerClient.FetchReadToken(ctx, req)
	if err != nil {
		return errors.Wrap(err, "repoManagerClient.FetchReadToken()")
	}
	c.gh = github.NewClient(nil).WithAuthToken(resp.GetToken())

	resolver := NewDeploymentResolver(c.gh, c.config)
	result, err := resolver.Resolve(ctx)
	if err != nil {
		return errors.Wrap(err, "resolver.Resolve()")
	}

	log.Printf("Resolved Deployment Configuration: %+v\n", result)

	return nil
}

// loadConfig loads the resolver configuration.
// Environment variables are always used for base config fields.
// If a JSON config file is provided, it loads the services array from it.
func (c *command) loadConfig(ctx context.Context) (*Config, error) {
	var cfg Config

	// Always load base config from environment variables
	log.Println("Loading base config from environment variables")
	if err := envconfig.Process(ctx, &cfg); err != nil {
		return nil, errors.Wrap(err, "envconfig.Process(Config)")
	}

	// If a config file is provided, load services from it
	if c.configFile != "" {
		log.Printf("Loading services from config file: %s", c.configFile)
		data, err := os.ReadFile(c.configFile)
		if err != nil {
			return nil, errors.Wrap(err, "os.ReadFile()")
		}

		// Parse only the services array from the JSON file
		var servicesConfig struct {
			Services []CloudRunService `json:"services"`
		}
		if err := json.Unmarshal(data, &servicesConfig); err != nil {
			return nil, errors.Wrap(err, "json.Unmarshal()")
		}
		cfg.Services = servicesConfig.Services
	}

	return &cfg, nil
}
