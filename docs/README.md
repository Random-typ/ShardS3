# ShardS3
`./src/shards3`
This directory contains the golang source files for ShardS3.
## Entrypoint
`./src/shards3/cmd/shards3`
This directory contains the entrypoint and some wrappers for the dashboard and the shards3 service.
## Modules
`./src/shards3/internal/modules`
This directory contains the `dashboard`, `s3` and `storage` modules.
### Dashboard
`./src/shards3/internal/modules/dashboard`
You can read more about this module [here](./Dashboard.md).

### S3
`./src/shards3/internal/modules/s3`
You can read more about this module [here](./S3.md).

### Storage
`./src/shards3/internal/modules/storage`
You can read more about this module [here](./Storage.md).

## Config
`./src/shards3/internal/platform`
This directory contains the code for the configuration and database.