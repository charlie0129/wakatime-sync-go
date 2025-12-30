# WakaTime Sync - Web Dashboard

Modern React-based dashboard for visualizing WakaTime coding statistics.

## Features

- 📊 Daily activity charts
- 🥧 Language, editor, and OS breakdown pie charts
- 📈 Project time distribution
- 📅 Flexible date range selection
- 🎨 Dark theme optimized for developers

## Development

```bash
# Install dependencies
npm install

# Start development server
npm run dev

# Build for production
npm run build
```

The development server runs on `http://localhost:5173` and proxies API requests to the Go backend on `http://localhost:3040`.

## Production

Build the frontend and serve it from the Go backend:

```bash
npm run build
```

The built files will be in `dist/` and served by the Go server.
