# WakaTime Sync (Go)

A lightweight Go application to sync and visualize your WakaTime coding statistics. Since WakaTime only allows free users to view their last 14 days of coding activity, this tool helps you store all your WakaTime data locally and visualize it with various charts. It fetches data from WakaTime's API and saves it in a local database every day, providing a dashboard for analysis.

This is a rewrite of the original Java/Spring version ([charlie0129/wakatime-sync](https://github.com/charlie0129/wakatime-sync), fork of [wf2311/wakatime-sync](https://github.com/wf2311/wakatime-sync)) using Go + SQLite (no CGO) + React.

<img width="2842" height="4434" alt="image" src="https://github.com/user-attachments/assets/7e97c808-f081-4bb3-b67d-efc86303b021" />

## Why a Rewrite?

It's a personal preference. I don't like Java/Spring that much and the old project is too heavy with too much dependencies. I want something simple and easy to manage. For example: 

- I don't want log rotation in the application, which should be handled by the Docker or Kubelet. Common sense. Don't do something that is not your responsibility.
- I don't want any external database dependency like MySQL. Harder to operate, consumes more resources, and overkill for such project. SQLite is a no-brainer, no second choice here.
- Spring + Java is just too heavy. Consumes a ton of memory. The image size is huge. I just cannot tolerate this thing on my personal server, wasting my resources. Just use Go, it's simple, efficient, and compiles to a single binary. That's hundreds of MBs to several MBs, over **10x reduction** right there, both image size and memory usage.
- I don't like Java. Come on, who likes Java nowadays?

Over the years, I have always wanted to rewrite the old project but never got around to it. Mostly because the old one still works (if it ain't broke, don't fix it) and I don't have that much time for this. Finally, with the help of AI/LLMs, I am able to get this project started and finished quickly without taking much time.

## AI Assistance Disclaimer

This project was developed with assistance from AI/LLMs, supervised and modified by humans (yes, me) who knew exactly what they were doing.

## Features

- 🔄 Automatic daily sync of WakaTime data
- 📊 REST API compatible with WakaTime's API format
- 💾 SQLite storage (pure Go, no CGO required)
- 📈 Modern React dashboard with charts
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

Copy the example config and edit it:

```bash
cp config.example.yaml config.yaml
```

Set `database_path` to `/app/data/wakatime.db` and fill in your WakaTime API key.

Then run:

```bash
docker run -d \
  -p 3040:3040 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/data:/app/data \
  ghcr.io/charlie0129/wakatime-sync-go
# or charlie0129/wakatime-sync-go
```

You can put whatever reverse proxy in front of it (Caddy, Nginx, Traefik, etc.) as needed. If you want to have basic auth, just put it in the reverse proxy middleware.

## Configuration Options

| Option             | Description                                  | Default       |
| ------------------ | -------------------------------------------- | ------------- |
| `listen_addr`      | Server listen address                        | `:3040`       |
| `database_path`    | SQLite database file path                    | `wakatime.db` |
| `wakatime_api_key` | Your WakaTime API key                        | required      |
| `proxy_url`        | HTTP/SOCKS5 proxy for WakaTime API           | empty         |
| `start_date`       | Start date for historical sync               | `2016-01-01`  |
| `sync_schedule`    | Cron schedule for auto sync                  | `0 1 * * *`   |
| `timezone`         | Timezone for date calculations and sync cron | `Local`       |

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

Trigger a manual sync for the last N days:

```bash
curl -X POST "http://localhost:3040/api/v1/sync?days=30&api_key=YOUR_API_KEY"
```

## API Endpoints

The API is designed to be compatible with the official WakaTime API format.

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

### Sync
```
POST /api/v1/sync?days=7&api_key=YOUR_API_KEY
GET /api/v1/sync/status
```

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
