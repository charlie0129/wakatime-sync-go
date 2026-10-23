# WakaTime Sync (Go)

A lightweight Go application to sync and visualize your WakaTime coding statistics. Since WakaTime only allows free users to view their last 14 days of coding activity, this tool helps you store all your WakaTime data locally and visualize it with various charts. It fetches data from WakaTime's API and saves it in a local database every day, providing a dashboard for analysis.

This is a rewrite of the original Java/Spring version ([charlie0129/wakatime-sync](https://github.com/charlie0129/wakatime-sync), fork of [wf2311/wakatime-sync](https://github.com/wf2311/wakatime-sync)) using Go + SQLite (no CGO) + React.

<img width="2766" height="5266" alt="image" src="https://github.com/user-attachments/assets/fd5e9779-73eb-43a1-9105-571d51805b9f" />

## Why a Rewrite?

It's a personal preference. I don't like Java/Spring that much and the old project is too heavy with too much dependencies. I want something simple and easy to manage. For example: 

- I don't want log rotation in the application, which should be handled by the Docker or Kubelet. Common sense. Don't do something that is not your responsibility.
- I don't want any external database dependency like MySQL. Harder to operate, consumes more resources, and overkill for such project. SQLite is a no-brainer, no second choice here. (But maybe using VictoriaMetrics for time-series data in the future?)
- Spring + Java is just too heavy. Consumes a ton of memory. The image size is huge. I just cannot tolerate this thing on my personal server, wasting my resources. Just use Go, it's simple, efficient, and compiles to a single binary. That's hundreds of MBs to just over 10 MB, over **10x reduction** right there, both image size and memory usage.
- I don't like Java. Come on, who likes Java nowadays?

Over the years, I have always wanted to rewrite the old project but never got around to it. Mostly because the old one still works (if it ain't broke, don't fix it) and I don't have that much time for this. Finally, with the help of AI/LLMs, I am able to get this project started and finished quickly without taking much time.

## AI Assistance Disclaimer

This project was developed with assistance from AI/LLMs, supervised and modified by humans (yes, me) who knew exactly what they were doing.

## Features

- 🔄 Automatic daily sync of WakaTime data
- 📊 REST API compatible with WakaTime's API format
- 🤖 AI coding stats: time, lines changed by AI and by hand, tokens, estimated cost, and sessions, per model, project and editor
- 💾 SQLite storage (pure Go, no CGO required)
- 📈 A dashboard of when you coded, where the time went, and how much of it was AI, for any date range, in light and dark
- 🐳 Docker-ready (with optimized cross-compilation)

## Migration from Old Project

If you have existing data in the old MySQL database ([charlie0129/wakatime-sync](https://github.com/charlie0129/wakatime-sync) or [wf2311/wakatime-sync](https://github.com/wf2311/wakatime-sync)), use the migration script:

> [!NOTE]
> Make sure your python is new enough (>=3.12) because we need JSONB in SQLite 3.45.0+.

```bash
cd scripts
python3 -m venv .venv
source .venv/bin/activate
pip3 install mysql-connector-python
python3 migrate.py \
  --mysql-host localhost \
  --mysql-port 3306 \
  --mysql-user wakatime \
  --mysql-password YOUR_PASSWORD \
  --mysql-db wakatime \
  --sqlite-path ../wakatime.db
```

## Docker

You can configure the application using either a config file or environment variables.

**Option 1: Using Environment Variables**

You can configure everything using environment variables without a config file:

```bash
docker run -d \
  -p 3040:3040 \
  -e WAKATIME_API_KEY=your_api_key_here \
  -e DATABASE_PATH=/app/data/wakatime.db \
  -e TZ=America/New_York \
  -v $(pwd)/data:/app/data \
  ghcr.io/charlie0129/wakatime-sync-go
```

For self-hosted WakaTime instances, you can also specify a custom base URL:

```bash
docker run -d \
  -p 3040:3040 \
  -e WAKATIME_API_KEY=your_api_key_here \
  -e WAKATIME_BASE_URL=https://your-wakatime-instance.com/api/v1 \
  -e DATABASE_PATH=/app/data/wakatime.db \
  -e TZ=America/New_York \
  -v $(pwd)/data:/app/data \
  ghcr.io/charlie0129/wakatime-sync-go
```

Or with Docker Compose:

```yaml
name: wakatime-sync-go
services:
  wakatime-sync-go:
    container_name: wakatime-sync-go
    image: ghcr.io/charlie0129/wakatime-sync-go
    restart: unless-stopped
    ports:
      - "3040:3040"
    environment:
      - WAKATIME_API_KEY=your_api_key_here
      - DATABASE_PATH=/app/data/wakatime.db
      - LISTEN_ADDR=:3040
      - START_DATE=2020-01-01
      - SYNC_SCHEDULE=0 1 * * *
      - TZ=America/New_York
      # Optional: for self-hosted WakaTime instances
      # - WAKATIME_BASE_URL=https://your-wakatime-instance.com/api/v1
    volumes:
      - ./data:/app/data
```

<details>
<summary>Option 2: Using Config File</summary>

Copy the example config and edit it:

```bash
cp config.example.yaml config.yaml
```

> [!IMPORTANT]
> Set `database_path` to `/app/data/wakatime.db` and fill in your WakaTime API key.

Then run:

```bash
docker run -d \
  -p 3040:3040 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/data:/app/data \
  ghcr.io/charlie0129/wakatime-sync-go
# or charlie0129/wakatime-sync-go
```

Or use this Docker Compose file:

```yaml
name: wakatime-sync-go
services:
  wakatime-sync-go:
    container_name: wakatime-sync-go
    image: ghcr.io/charlie0129/wakatime-sync-go
    restart: unless-stopped
    ports:
      - "3040:3040"
    volumes:
      - ./config.yaml:/app/config.yaml
      - ./data:/app/data
```

</details>
<br />


You can put whatever reverse proxy in front of it (Caddy, Nginx, Traefik, etc.) as needed. If you want to have basic auth, just put it in the reverse proxy middleware.

It syncs on startup, then daily at 1:00 AM (using the `sync_schedule` and `timezone` from config). Each sync covers:

- on a fresh database, every day since `start_date`;
- otherwise, every day since the last successful sync, plus the last `sync_lookback_days` days again (WakaTime keeps receiving data for past days, e.g. from plugins that were offline), plus every day whose last sync failed.

A re-sync never replaces stored data with less data (less time or fewer heartbeats), so your archive is kept even if WakaTime stops serving old data. Such days are logged with a warning.

If you want to trigger a manual sync of last N days, or of a date range, use the API:

```bash
curl -X POST "http://localhost:3040/api/v1/sync?days=30&api_key=YOUR_API_KEY"
curl -X POST "http://localhost:3040/api/v1/sync?start=2025-01-01&end=2025-12-31&api_key=YOUR_API_KEY"
```

Add `force=true` to overwrite stored data even when WakaTime returns less of it. Only one sync runs at a time; the API returns `409 Conflict` while another one is running.

AI coding stats are synced since they were added to this project. To fill them in for days synced before, sync those days again, for example:

```bash
curl -X POST "http://localhost:3040/api/v1/sync?start=2025-06-01&api_key=YOUR_API_KEY"
```

## Configuration Options

Configuration can be provided via YAML file or environment variables. Environment variables take precedence over config file values.

| Option               | Environment Variable | Description                                       | Default                       |
| -------------------- | -------------------- | ------------------------------------------------- | ----------------------------- |
| `listen_addr`        | `LISTEN_ADDR`        | Server listen address                             | `:3040`                       |
| `database_path`      | `DATABASE_PATH`      | SQLite database file path                         | `wakatime.db`                 |
| `wakatime_api_key`   | `WAKATIME_API_KEY`   | Your WakaTime API key                             | required                      |
| `wakatime_base_url`  | `WAKATIME_BASE_URL`  | WakaTime API base URL (for self-hosted instances) | `https://wakatime.com/api/v1` |
| `proxy_url`          | `PROXY_URL`          | HTTP/SOCKS5 proxy for WakaTime API                | empty                         |
| `start_date`         | `START_DATE`         | Start date for historical sync                    | `2016-01-01`                  |
| `sync_schedule`      | `SYNC_SCHEDULE`      | Cron schedule for auto sync                       | `0 1 * * *`                   |
| `sync_lookback_days` | `SYNC_LOOKBACK_DAYS` | Number of recent days every sync fetches again    | `3`                           |
| `timezone`           | `TZ`                 | Timezone for date calculations and sync cron      | `Local`                       |

If you want to skip the initial sync on startup, set `SKIP_INITIAL_SYNC=true` environment variable.

To find your timezone string, refer to the list of [IANA Time Zone database names](https://en.wikipedia.org/wiki/List_of_tz_database_time_zones).

## Development Setup

### 1. Configuration

Copy the example config and edit it:

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml` with your WakaTime API key (get it from https://wakatime.com/settings/api-key):

```yaml
listen_addr: ":3040"
database_path: "wakatime.db"
wakatime_api_key: "YOUR_API_KEY"
start_date: "2020-01-01"
```

### 2. Build & Run

```bash
# Download dependencies
go mod tidy

# Build
go build -o wakatime-sync .

# Run
./wakatime-sync -config config.yaml
```

### 3. Frontend

```bash
cd web
npm install
npm run build
```

The built frontend will be served automatically by the Go server.

### 4. Sync Historical Data

On a fresh database, the first sync fetches every day since `start_date`. You can also trigger a manual sync for the last N days, or for a date range:

```bash
curl -X POST "http://localhost:3040/api/v1/sync?days=30&api_key=YOUR_API_KEY"
curl -X POST "http://localhost:3040/api/v1/sync?start=2025-01-01&end=2025-12-31&api_key=YOUR_API_KEY"
```

## API Endpoints

The API is designed to be compatible with the official WakaTime API format, including the AI coding fields of durations, heartbeats, and summaries (`ai_additions`, `human_additions`, `ai_input_tokens`, `ai_model_total_cost`, `ai_model_breakdown`, etc.).

### Durations
```
GET /api/v1/users/current/durations?date=2024-01-15
GET /api/v1/users/current/durations?date=2024-01-15&project=myproject
```

### Heartbeats
```
GET /api/v1/users/current/heartbeats?date=2024-01-15
```

### Summaries
```
GET /api/v1/users/current/summaries?start=2024-01-01&end=2024-01-31
```

### Projects
```
GET /api/v1/users/current/projects
GET /api/v1/users/current/projects?q=search
```

### Additional Stats Endpoints
```
GET /api/v1/stats/daily?start=2024-01-01&end=2024-01-31
GET /api/v1/stats/range?start=2024-01-01&end=2024-01-31
```

`/stats/range` also returns AI coding totals (`ai`), the usage of each AI model (`ai_models`), the token usage of each AI model version (`ai_model_versions`), and every AI model ever used, most expensive first (`ai_model_names`).

### Dashboard
```
GET /api/v1/dashboard?range=30d
GET /api/v1/dashboard?start=2024-01-01&end=2024-01-31
GET /api/v1/activity?range=30d
```

The web dashboard gets everything it shows for a date range from `/dashboard` in one response, including the previous period of the same length to compare with, and when coding happened on each day from `/activity`. `range` is one of `yesterday`, `7d`, `30d`, `90d`, `12m`, `this-week`, `last-week`, `this-month`, `last-month`, `this-year` and `last-year`, in the configured timezone. These responses follow the dashboard and may change with it.

### Sync
```
POST /api/v1/sync?days=7&api_key=YOUR_API_KEY
POST /api/v1/sync?start=2024-01-01&end=2024-01-31&force=true&api_key=YOUR_API_KEY
GET /api/v1/sync/status
```

The status has the last successfully synced day, the days whose last sync failed, and the current sync run, or else the last one.

## Project Structure

```
wakatime-sync-go/
├── main.go                 # Application entry point
├── config.yaml             # Configuration file
├── go.mod                  # Go module definition
├── internal/
│   ├── api/               # HTTP handlers
│   ├── config/            # Configuration loading
│   ├── database/          # SQLite database operations
│   ├── models/            # Data models
│   ├── sync/              # WakaTime sync logic
│   └── wakatime/          # WakaTime API client
├── scripts/
│   └── migrate.py         # MySQL to SQLite migration
└── web/                   # React frontend
    ├── src/
    └── dist/              # Built frontend (after npm run build)
```
