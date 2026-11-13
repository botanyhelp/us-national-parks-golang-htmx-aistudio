# U.S. National Parks Explorer (Go & htmx)

A high-performance Go web application that downloads U.S. National Park records directly from Wikipedia into memory, featuring a responsive master-detail UI built with **htmx** (zero heavy JS frameworks), packaged in a lightweight Alpine container for **GCP Cloud Run**.

## Architecture & Features

- **100% Standard Library Go:** Uses only Go standard libraries (`net/http`, `html/template`, `regexp`, `strings`, `sync`) with zero third-party Go dependencies.
- **In-Memory Store:** Scrapes and parses the Wikipedia table on startup, caching records in a thread-safe `ParkStore` with fallback seed data for offline safety.
- **htmx UI Interactivity:**
  - **Live Search & Filtering:** Debounced input triggers instant search across park names, states, and descriptions.
  - **Dynamic Detail Pane:** Clicking any park fetches and renders the full-resolution image, statistics, description, and Wikipedia/Google Maps links into the detail view without page reloads.
  - **Manual Sync:** Sync button re-fetches live Wikipedia data on demand.
- **Cloud Run Ready:** Binds to `0.0.0.0:$PORT`, handles graceful termination (`SIGTERM`), and exposes `/healthz`.

## Local Development (Go)

```bash
go run main.go
```

## Docker Container Build & Run

```bash
docker build -t us-national-parks .
docker run -p 8080:8080 us-national-parks
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
