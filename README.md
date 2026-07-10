# ShardS3

## Run
```
podman compose -f .\container\compose.yaml up
```
## Modules

- `services/shards3`: Primary S3-compatible API service (hello world bootstrap + endpoint/module stubs).
- `services/dashboard`: Future statistics API for dashboard/frontend consumption.

## Planned S3 Feature Areas

- Bucket CRUD
- Object CRUD
- ListObjectsV2
- Multipart uploads
- HEAD requests
- SigV4 authentication
- Presigned URLs

## Notes

This repository currently contains boilerplate only. No production S3 logic is implemented yet.

## Storage
