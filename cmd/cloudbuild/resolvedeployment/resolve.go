// Package resolvedeployment provides logic for resolving Cloud Build deployment targets.
package resolvedeployment

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/errors/v5"
	"github.com/google/go-github/v80/github"
)

const (
	prodEnv               = "prd"
	environmentScriptFile = "environment.sh"
)

// CloudRunService defines the configuration for a single Cloud Run service.
type CloudRunService struct {
	// Name is the Cloud Run service name.
	Name string `json:"name"`
	// Repository is the Artifact Registry repository URL.
	Repository string `json:"repository"`
	// ImageName is the container image name.
	ImageName string `json:"imageName"`
	// Subdomain is an optional template for the service subdomain (may contain APPCODE_PLACEHOLDER).
	Subdomain string `json:"subdomain,omitempty"`
	// OIDCRedirectPath is an optional OIDC redirect path for authentication.
	OIDCRedirectPath string `json:"oidcRedirectPath,omitempty"`
}

// Config holds the input configuration for resolving a deployment target.
type Config struct {
	// AppCode is the base application code (e.g., "app").
	AppCode string `env:"_APP_CODE"`
	// SpannerDatabaseName is the default Spanner database name.
	SpannerDatabaseName string `env:"_GOOGLE_CLOUD_SPANNER_DATABASE_NAME"`
	// TagName is the Git tag name if triggered by a tag (empty otherwise).
	TagName string `env:"TAG_NAME"`
	// PRNumber is the pull request number if triggered by a PR (0 otherwise).
	PRNumber int `env:"_PR_NUMBER"`
	// CommitSHA is the commit SHA of the current build.
	CommitSHA string `env:"COMMIT_SHA"`
	// DefaultBranch is the default branch name (e.g., "master" or "main").
	DefaultBranch string `env:"DEFAULT_BRANCH"`
	// RepoName is the repository name (e.g., "app-app").
	RepoName string `env:"REPO_NAME"`
	// RepoFullName is the full name for the repository (e.g., "cccteam/app-app").
	RepoFullName string `env:"REPO_FULL_NAME"`
	// FeatureTestingCustomDBs is a list of instance numbers that use custom databases.
	FeatureTestingCustomDBs []int `env:"_FEATURE_TESTING_CUSTOM_DBS"`
	// FeatureTestingSpannerDatabaseName is the template for custom database names.
	// Should contain "APPCODE_PLACEHOLDER" which will be replaced with the target app code.
	FeatureTestingSpannerDatabaseName string `env:"_FEATURE_TESTING_SPANNER_DATABASE_NAME"`
	// AppEnv is the application environment (e.g., "prd", "stg", "tst").
	AppEnv string `env:"_APP_ENV"`
	// AppPWAName is the name of the PWA application.
	AppPWAName string `env:"_APP_PWA_NAME"`
	// AppPWAShortName is the short name of the PWA application.
	AppPWAShortName string `env:"_APP_PWA_SHORT_NAME"`
	// Services is the list of Cloud Run services to deploy.
	Services []CloudRunService
}

func (c *Config) RepoOwner() string {
	parts := strings.SplitN(c.RepoFullName, "/", 2)
	if len(parts) != 2 {
		return ""
	}

	return parts[0]
}

// ResolvedService holds the resolved configuration for a single Cloud Run service.
type ResolvedService struct {
	// Name is the Cloud Run service name.
	Name string
	// ImageURL is the fully qualified container image URL with tag.
	ImageURL string
	// OIDCRedirectURL is the resolved OIDC redirect URL (if applicable).
	OIDCRedirectURL string
}

// Result holds the resolved deployment target configuration.
type Result struct {
	// TargetAppCode is the resolved application code (may include instance number for PR builds).
	TargetAppCode string
	// SpannerDatabaseName is the resolved Spanner database name.
	SpannerDatabaseName string
	// AppPWAName is the resolved PWA name (may include environment suffix).
	AppPWAName string
	// AppPWAShortName is the resolved PWA short name.
	AppPWAShortName string
	// DisableEmailWhitelist indicates whether email whitelisting should be disabled.
	DisableEmailWhitelist bool
	// Services is the list of resolved Cloud Run service configurations.
	Services []ResolvedService
	// TemplateServiceNames is a comma-separated list of service names.
	TemplateServiceNames string
	// TemplateImageURLs is a comma-separated list of image URLs.
	TemplateImageURLs string
}

// DeploymentResolver resolves deployment targets based on build triggers.
type DeploymentResolver struct {
	github *github.Client
	cfg    *Config
}

// NewDeploymentResolver creates a new Resolver with the given GitHub client.
func NewDeploymentResolver(ghClient *github.Client, cfg *Config) *DeploymentResolver {
	return &DeploymentResolver{
		github: ghClient,
		cfg:    cfg,
	}
}

// Resolve determines the deployment target based on the build configuration.
func (r *DeploymentResolver) Resolve(ctx context.Context) (*Result, error) {
	result := &Result{
		TargetAppCode:         r.cfg.AppCode,
		SpannerDatabaseName:   r.cfg.SpannerDatabaseName,
		AppPWAName:            r.cfg.AppPWAName,
		AppPWAShortName:       r.cfg.AppPWAShortName,
		DisableEmailWhitelist: r.cfg.AppEnv == prodEnv,
		Services:              []ResolvedService{},
		TemplateServiceNames:  "",
		TemplateImageURLs:     "",
	}

	// Handle triggers from Git tags or Pull Requests
	if r.cfg.TagName != "" || r.cfg.PRNumber != 0 {
		if r.cfg.TagName != "" {
			log.Printf("Build triggered by Tag detected. Tag name: %s\n", r.cfg.TagName)
			if err := r.resolveTagBuild(ctx); err != nil {
				return nil, errors.Wrap(err, "Resolver.resolveTagBuild()")
			}
		} else if r.cfg.PRNumber != 0 {
			log.Printf("Build triggered by Pull Request detected. PR number: %d\n", r.cfg.PRNumber)
			targetAppCode, spannerDatabaseName, err := r.resolvePRBuild(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "Resolver.resolvePRBuild()")
			}
			result.TargetAppCode = targetAppCode
			if spannerDatabaseName != "" {
				result.SpannerDatabaseName = spannerDatabaseName
			}
		}
	}

	// Update PWA names for non-production environments
	if r.cfg.AppEnv != prodEnv {
		result.AppPWAName = fmt.Sprintf(
			"%s (%s.%s)",
			r.cfg.AppPWAName,
			result.TargetAppCode,
			r.cfg.AppEnv,
		)
		result.AppPWAShortName = fmt.Sprintf("%s.%s", result.TargetAppCode, r.cfg.AppEnv)
		log.Printf(
			"Updated PWA names for non-production environment: APP_PWA_NAME=%s, APP_PWA_SHORT_NAME=%s\n",
			result.AppPWAName,
			result.AppPWAShortName,
		)
	}

	// Resolve Cloud Run services
	serviceNames := make([]string, 0, len(r.cfg.Services))
	imageURLs := make([]string, 0, len(r.cfg.Services))
	for _, svc := range r.cfg.Services {
		imageURL := fmt.Sprintf("%s/%s:%s", svc.Repository, svc.ImageName, r.cfg.CommitSHA)
		resolved := ResolvedService{
			Name:     svc.Name,
			ImageURL: imageURL,
		}
		// Resolve OIDC redirect URL if subdomain and path are configured
		if svc.Subdomain != "" && svc.OIDCRedirectPath != "" {
			resolvedSubdomain := strings.ReplaceAll(
				svc.Subdomain,
				"APPCODE_PLACEHOLDER",
				result.TargetAppCode,
			)
			resolved.OIDCRedirectURL = resolvedSubdomain + "/" + svc.OIDCRedirectPath
		}
		result.Services = append(result.Services, resolved)
		serviceNames = append(serviceNames, svc.Name)
		imageURLs = append(imageURLs, imageURL)
	}
	result.TemplateServiceNames = strings.Join(serviceNames, ",")
	result.TemplateImageURLs = strings.Join(imageURLs, ",")

	if err := writeEnvironmentScript(result); err != nil {
		return nil, errors.Wrap(err, "writeEnvironmentScript()")
	}

	return result, nil
}

// resolveTagBuild validates that a tag is on the tip of the default branch.
func (r *DeploymentResolver) resolveTagBuild(ctx context.Context) error {
	// Use the GitHub API to check if the commit is on the default branch
	comparison, _, err := r.github.Repositories.CompareCommits(
		ctx,
		r.cfg.RepoOwner(),
		r.cfg.RepoName,
		r.cfg.DefaultBranch,
		r.cfg.CommitSHA,
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "github.Repositories.CompareCommits()")
	}

	// If the commit is the head of the default branch, BehindBy should be 0
	// and Status should be "identical" or "ahead"
	if comparison.GetBehindBy() != 0 {
		return fmt.Errorf(
			"build REJECTED: Tag is not on tip of %s branch (behind by %d commits)",
			r.cfg.DefaultBranch,
			comparison.GetBehindBy(),
		)
	}

	return nil
}

// resolvePRBuild resolves the deployment target for a PR-triggered build.
// It fetches PR comments to find the latest /gcbrun command.
// The command should be in the format: /gcbrun <numeric_value>
// Additionally, the
func (r *DeploymentResolver) resolvePRBuild(
	ctx context.Context,
) (targetAppCode, spannerDatabaseName string, err error) {
	twentyfourHoursAgo := time.Now().Add(-time.Hour * 24)
	sort := "created"
	direction := "desc"
	opts := &github.IssueListCommentsOptions{
		Sort:      &sort,
		Direction: &direction,
		Since:     &twentyfourHoursAgo, // Fetch comments from the last 24 hours
		ListOptions: github.ListOptions{
			PerPage: 100,
			Page:    1,
		},
	}

	comments, _, err := r.github.Issues.ListComments(
		ctx,
		r.cfg.RepoOwner(),
		r.cfg.RepoName,
		r.cfg.PRNumber,
		opts,
	)
	if err != nil {
		return "", "", errors.Wrap(err, "github.Issues.ListComments()")
	}

	// Find the latest /gcbrun comment
	instanceNumber, err := parseGCBRunComment(comments)
	if err != nil {
		return "", "", errors.Wrap(err, "parseGCBRunComment()")
	}

	// Construct the final TargetAppCode
	targetAppCode = fmt.Sprintf("%s%d", r.cfg.AppCode, instanceNumber)
	log.Printf(
		"Resolved TargetAppCode=%s from /gcbrun command in PR #%d\n",
		targetAppCode,
		r.cfg.PRNumber,
	)
	// Check if this instance uses a custom database
	if len(r.cfg.FeatureTestingCustomDBs) > 0 {
		zeroIndexedInstance := instanceNumber - 1

		if slices.Contains(r.cfg.FeatureTestingCustomDBs, zeroIndexedInstance) {
			spannerDatabaseName = strings.ReplaceAll(
				r.cfg.FeatureTestingSpannerDatabaseName,
				"APPCODE_PLACEHOLDER",
				targetAppCode,
			)

			log.Printf(
				"INSTANCE_NUMBER=%d found in _FEATURE_TESTING_CUSTOM_DBS=%v. Updating GOOGLE_CLOUD_SPANNER_DATABASE_NAME=%s\n",
				instanceNumber,
				r.cfg.FeatureTestingCustomDBs,
				spannerDatabaseName,
			)
		}
	}

	return targetAppCode, spannerDatabaseName, nil
}

// parseGCBRunComment finds and parses the latest /gcbrun comment.
func parseGCBRunComment(comments []*github.IssueComment) (int, error) {
	var latestBody string

	// Find the last comment starting with "/gcbrun"
	for _, c := range comments {
		body := c.GetBody()
		if strings.HasPrefix(body, "/gcbrun") {
			latestBody = body
		}
	}

	if latestBody == "" {
		return 0, fmt.Errorf("no /gcbrun comment found in the last 24 hours")
	}

	log.Printf("Found comment: %s\n", latestBody)

	// Extract the numeric instance identifier (e.g., "123" from "/gcbrun 123")
	parts := strings.Fields(latestBody)
	if len(parts) < 2 {
		return 0, fmt.Errorf(
			"no valid environment number found in comment: %s. The command should be in the format: /gcbrun <numeric_value>",
			latestBody,
		)
	}

	if !regexp.MustCompile(`^\d+$`).MatchString(parts[1]) {
		return 0, fmt.Errorf(
			"no valid environment number found in comment: %s. The command should be in the format: /gcbrun <numeric_value>",
			latestBody,
		)
	}

	instanceNumber, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf(
			"no valid environment number found in comment: %s. The command should be in the format: /gcbrun <numeric_value>",
			latestBody,
		)
	}

	return instanceNumber, nil
}

// writeEnvironmentScript creates an environment script with the resolved result.
func writeEnvironmentScript(result *Result) error {
	var sb strings.Builder
	sb.WriteString(`#!/bin/bash
set -euo pipefail
`)
	fmt.Fprintf(&sb, `export TARGET_APP_CODE="%s"
`, result.TargetAppCode)
	fmt.Fprintf(&sb, `export GOOGLE_CLOUD_SPANNER_DATABASE_NAME="%s"
`, result.SpannerDatabaseName)
	fmt.Fprintf(&sb, `export APP_PWA_NAME="%s"
`, result.AppPWAName)
	fmt.Fprintf(&sb, `export APP_PWA_SHORT_NAME="%s"
`, result.AppPWAShortName)
	fmt.Fprintf(&sb, `export APP_DISABLE_EMAIL_WHITELIST="%t"
`, result.DisableEmailWhitelist)
	fmt.Fprintf(&sb, `export _template_service_names="%s"
`, result.TemplateServiceNames)
	fmt.Fprintf(&sb, `export _template_image_urls="%s"
`, result.TemplateImageURLs)

	// Write per-service OIDC redirect URLs
	for _, svc := range result.Services {
		if svc.OIDCRedirectURL != "" {
			envVarName := strings.ToUpper(
				strings.ReplaceAll(svc.Name, "-", "_"),
			) + "_OIDC_REDIRECT_URL"
			fmt.Fprintf(&sb, `export %s="%s"
`, envVarName, svc.OIDCRedirectURL)
		}
	}

	if err := os.WriteFile(environmentScriptFile, []byte(sb.String()), 0o600); err != nil {
		return errors.Wrap(err, "os.WriteFile()")
	}

	return nil
}
