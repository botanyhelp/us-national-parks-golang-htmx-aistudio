# U.S. National Parks Explorer (Go & htmx)

A lightweight, high-performance web application that fetches and parses U.S. National Park records directly from Wikipedia into memory. It features a responsive master-detail interface powered by **htmx** (no client-side JS framework required) and is packaged in a minimal multi-stage Alpine Docker container ready for **GCP Cloud Run**.

## Features

- **Zero External Go Dependencies:** Built entirely with the Go 1.22 standard library (`net/http`, `html/template`, `regexp`, `sync`).
- **In-Memory Data Store (`ParkStore`):** Scrapes and parses the [List of national parks of the United States](https://en.wikipedia.org/wiki/List_of_national_parks_of_the_United_States) Wikipedia table on startup, storing records in a thread-safe (`sync.RWMutex`) in-memory cache with automatic fallback seed data if offline.
- **Interactive htmx UI:**
  - **Live Search:** Debounced search (`200ms`) filters parks by name, state, or description without full page reloads.
  - **Master-Detail View:** Selecting a park dynamically loads its hero image, establishment date, area, visitor count, coordinates, UNESCO designations (World Heritage Site / Biosphere Reserve), and external links (Wikipedia & Google Maps).
  - **On-Demand Wikipedia Sync:** Manual refresh button (`POST /parks/refresh`) re-syncs the in-memory store with live Wikipedia data.
- **Dual Runtime Support:**
  - **Production / Go (`main.go`):** Statically compiled Go binary for production and Docker deployments.
  - **Node.js / TypeScript (`server.ts`):** Express + `tsx` development server for environments without a Go toolchain (e.g., AI Studio preview).
- **Cloud Run Ready:** Listens on `$PORT` (default `8080`), supports graceful shutdown on `SIGINT`/`SIGTERM`, and provides a `/healthz` endpoint.

## Project Structure

```text
.
├── main.go          # Primary Go HTTP server, Wikipedia parser, and htmx templates
├── go.mod           # Go module definition (Go 1.22)
├── Dockerfile       # Multi-stage build (golang:1.22-alpine -> alpine:3.19)
├── server.ts        # Alternative Node/Express + htmx server for preview environments
└── package.json     # Node.js scripts and dependencies for server.ts
```

## HTTP Endpoints

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/` | Renders the full application shell and initial park selection |
| `GET` | `/parks/search?q=<query>` | Returns an HTML fragment (`park-items`) matching the search query |
| `GET` | `/parks/detail?id=<slug>` | Returns an HTML fragment (`park-detail`) for a specific park |
| `POST` | `/parks/refresh` | Re-fetches data from Wikipedia and returns the updated `#app-view` fragment |
| `GET` | `/healthz` | Returns JSON health status, loaded park count, and last sync timestamp |

## Getting Started

### Option 1: Run with Go (Recommended)

Requires **Go 1.22+**:

```bash
go run main.go
```

The server will start on [http://localhost:8080](http://localhost:8080) (or the port specified by `$PORT`).

### Option 2: Run with Node.js / Bun (Development Preview)

```bash
npm install
npm run dev
```

Starts the Express development server from `server.ts` on [http://localhost:3000](http://localhost:3000).

### Option 3: Build & Run with Docker

```bash
docker build -t us-national-parks .
docker run --rm -p 8080:8080 us-national-parks
```

## Deploy to GCP Cloud Run

```bash
gcloud run deploy us-national-parks \
  --source . \
  --platform managed \
  --region us-central1 \
  --allow-unauthenticated \
  --memory 256Mi \
  --cpu 1
```
