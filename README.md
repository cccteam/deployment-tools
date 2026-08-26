# deployment-tools

This repository provides a CLI tool for managing actions that happen during a deployment

## Overview

The CLI is currently designed to help automate and safely manage schema and data migrations for Spanner databases. It provides commands for:

- **Bootstrapping** a database (applying schema and data migrations)
- **Dropping** all schema tables (with safety checks to prevent accidental use in production)
- **Staging** a database (backing up a source database and restoring it to a target database)

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

### Stage DB

```sh
deployment-tools db spanner stagedb <target_database>
```

- Backs up the source database and restores it to the target database.
- The source database comes from the `GOOGLE_CLOUD_SPANNER_DATABASE_NAME` environment variable.
- The target database name is passed as an argument to the command.
- `--backupOnly` (`-b`) takes the backup and stops there, skipping the restore. A target argument is still required but is not used.

**NOTES:**
- The command will fail if the target database name contains `prd` or `prod`.
- The target database name is lowercased before the restore, so `MyStageDb` creates `mystagedb`.
- The target database *must not exist* when the restore starts. Drop it before running this command.
- `GOOGLE_CLOUD_SPANNER_DATABASE_MAX_AGE` sets how old an existing backup may be, in seconds. If the most recent backup of the source database is newer than this, it is reused; otherwise a new backup is taken.

## Environment Variables

The following environment variables must be set to connect to your Spanner instance:

- `GOOGLE_CLOUD_SPANNER_PROJECT`
- `GOOGLE_CLOUD_SPANNER_INSTANCE_ID`
- `GOOGLE_CLOUD_SPANNER_DATABASE_NAME`

`stagedb` also requires:

- `GOOGLE_CLOUD_SPANNER_DATABASE_MAX_AGE` - maximum age of an existing backup, in seconds, before a new source backup is taken

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

# Back up my-db and restore it to my-stage-db, reusing a backup taken in the last hour
export GOOGLE_CLOUD_SPANNER_DATABASE_MAX_AGE=3600
deployment-tools db spanner stagedb my-stage-db
```

## Safety

- The drop command will refuse to run if `_APP_ENV` indicates a production environment.
- All operations use the [migrate](https://github.com/zredinger-ccc/migrate) library for safe, repeatable migrations.

