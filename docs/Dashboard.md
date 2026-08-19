# Dashboard

`./src/shards3/internal/modules/dashboard`

The Dashboard is a web-based management interface for monitoring and administering ShardS3 storage operations. It provides real-time visibility into cluster health, backend storage usage, and S3 bucket management.

## Architecture

The dashboard is built with Go's standard `net/http` library and uses server-side template rendering. It consists of two main components:

- **Service** (`service.go`): Business logic layer that queries the metadata database and aggregates statistics
- **Web Server** (`web/server.go`): HTTP request handler and template rendering engine

The service runs as a separate goroutine alongside the S3 module and is configured via the application's central config using the `DashboardAddress` setting.

## Key Features

### Monitoring & Visualization
- **Health Checks**: Periodic database connectivity verification with long-lived polling endpoint (`/health/longlived`)
- **Backend Statistics**: Per-backend monitoring showing:
  - Shard count and distribution (configured vs. unconfigured backends tracked separately)
  - Total bytes stored and relative usage percentages
  - Last verification timestamp
  - Maximum shard size constraints
  - Separate handling for backends registered in config vs. orphaned backends with metadata records

### Bucket Management
- List all buckets with aggregated statistics (object count, total size)
- Create and delete buckets via API endpoints
- Fallback to simple bucket listing if statistics aggregation fails

### Object Browser
- Hierarchical object browsing by prefix with directory-like navigation
- Displays object metadata including:
  - Size, last modified time
  - Chunk and shard count (indicating fragmentation)
  - Automatic directory aggregation (objects sharing common prefixes rendered as folders)

## Implementation Details

### Static Asset Embedding
All static files (CSS, JavaScript) and Go HTML templates are embedded at compile time using the `//go:embed` directive. This ensures the dashboard is fully self-contained and requires no external file dependencies.

### Template System
- Uses Go's `html/template` package for safe HTML rendering
- Custom template functions for formatting:
  - `formatBytes`: Human-readable byte sizes (B, KiB, MiB, GiB, etc.)
  - `formatTime`: Relative time display for verification timestamps
- Fragment-based routing supports partial page updates (`/fragments/*` endpoints) for HTMX-style interactivity

### Routing
The dashboard exposes the following routes:
- `GET /` → Home/dashboard page
- `GET /buckets` → Bucket listing and management
- `GET /backends` → Backend statistics and health
- `GET /settings` → Configuration view
- `GET /health/longlived` → Long-polling health check endpoint
- `GET /fragments/*` → Partial page updates (HTMX endpoints)
- `POST /fragments/*` → Bucket creation/deletion operations
- `GET /static/*` → Embedded CSS, JavaScript, and assets

### Database Integration
The dashboard queries the SQLite metadata database (`internal/platform/db`) for:
- Bucket enumeration and statistics
- Object listings and metadata
- Backend shard/chunk/byte aggregations
- Chunk and multipart upload tracking
