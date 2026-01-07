# deployment-tools

This repository provides a CLI tool for managing database operations during deployments, with a focus on Google Cloud Spanner.

## Overview

The CLI is designed to help automate and safely manage schema and data migrations for Spanner databases. It provides commands for:

- **Bootstrapping** a database (applying schema and data migrations)
- **Dropping** all schema tables (with safety checks to prevent accidental use in production)

## DB Command Structure

The CLI organizes database commands under the `db` command group, with Spanner-specific operations under `db spanner`. The main subcommands are:

### Bootstrap

```sh
deployment-tools db spanner bootstrap --schema-dir <schema-migrations-dir> --data-dirs <data-migrations-dir1>,<data-migrations-dir2>
```

- Applies all schema migrations from the specified directory.
- Runs data migrations from one or more directories.
- Uses environment variables to connect to the target Spanner database.

### Drop Schema

```sh
deployment-tools db spanner drop --schema-dir <schema-migrations-dir>
```

- Drops all tables defined in the db.
- **Safety:** Will not run if the `_APP_ENV` environment variable is set to `prd`, `prod`, or `production`.

## Environment Variables

The following environment variables must be set to connect to your Spanner instance:

- `GOOGLE_CLOUD_SPANNER_PROJECT`
- `GOOGLE_CLOUD_SPANNER_INSTANCE_ID`
- `GOOGLE_CLOUD_SPANNER_DATABASE_NAME`

## Example Usage

```sh
export GOOGLE_CLOUD_SPANNER_PROJECT=my-gcp-project
export GOOGLE_CLOUD_SPANNER_INSTANCE_ID=my-instance
export GOOGLE_CLOUD_SPANNER_DATABASE_NAME=my-db
export _APP_ENV=dev

# Bootstrap the database
deployment-tools db spanner bootstrap

# Drop all tables (not allowed in production)
deployment-tools db spanner drop
```

## Safety

- The drop command will refuse to run if `_APP_ENV` indicates a production environment.
- All operations use the [migrate](https://github.com/zredinger-ccc/migrate) library for safe, repeatable migrations.

