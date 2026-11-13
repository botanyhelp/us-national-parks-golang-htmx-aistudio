package main

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

const wikiURL = "https://en.wikipedia.org/wiki/List_of_national_parks_of_the_United_States"

// Park represents a U.S. National Park record.
type Park struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	Location    string `json:"location"`
	DateEst     string `json:"date_established"`
	Area        string `json:"area"`
	Visitors    string `json:"visitors"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url"`
	WikiURL     string `json:"wiki_url"`
	IsWHS       bool   `json:"is_whs"` // UNESCO World Heritage Site
	IsBR        bool   `json:"is_br"`  // UNESCO Biosphere Reserve
}

// ParkStore manages in-memory park records.
type ParkStore struct {
	sync.RWMutex
	parks      []Park
	parksByID  map[string]Park
	lastLoaded time.Time
	loadErr    error
}

func NewParkStore() *ParkStore {
	return &ParkStore{
		parks:     make([]Park, 0),
		parksByID: make(map[string]Park),
	}
}

func (s *ParkStore) LoadFromWikipedia(url string) error {
	client := &http.Client{Timeout: 35 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Wikipedia requires a custom User-Agent policy header
	req.Header.Set("User-Agent", "USNationalParksViewer/1.0 (Golang HTMX Educational Demo; contact@example.com)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching wikipedia url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wikipedia returned status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	parsed, err := parseWikipediaHTML(string(body))
	if err != nil {
		return fmt.Errorf("parsing table: %w", err)
	}

	s.Lock()
	defer s.Unlock()
	s.parks = parsed
	s.parksByID = make(map[string]Park, len(parsed))
	for _, p := range parsed {
		s.parksByID[p.ID] = p
	}
	s.lastLoaded = time.Now()
	s.loadErr = nil
	return nil
}

func (s *ParkStore) GetAll() []Park {
	s.RLock()
	defer s.RUnlock()
	res := make([]Park, len(s.parks))
	copy(res, s.parks)
	return res
}

func (s *ParkStore) GetByID(id string) (Park, bool) {
	s.RLock()
	defer s.RUnlock()
	p, ok := s.parksByID[id]
	return p, ok
}

func (s *ParkStore) Search(query string) []Park {
	s.RLock()
	defer s.RUnlock()
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		res := make([]Park, len(s.parks))
		copy(res, s.parks)
		return res
	}

	var results []Park
	for _, p := range s.parks {
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.State), q) ||
			strings.Contains(strings.ToLower(p.Description), q) {
			results = append(results, p)
		}
	}
	return results
}

func (s *ParkStore) Status() (int, time.Time, error) {
	s.RLock()
	defer s.RUnlock()
	return len(s.parks), s.lastLoaded, s.loadErr
}

// ----------------- HTML Parser (Standard Library Only) -----------------

var (
	tagRegex = regexp.MustCompile(`(?s)<[^>]*>`)
	refRegex = regexp.MustCompile(`\[[0-9a-zA-Z\s,]+\]`)
)

func cleanText(s string) string {
	s = refRegex.ReplaceAllString(s, "")
	s = tagRegex.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' {
			b.WriteRune('-')
		}
	}
	res := strings.Trim(b.String(), "-")
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	return res
}

func parseWikipediaHTML(pageHTML string) ([]Park, error) {
	tableStart := strings.Index(pageHTML, "<table")
	var targetTable string

	for tableStart != -1 {
		tableEnd := strings.Index(pageHTML[tableStart:], "</table>")
		if tableEnd == -1 {
			break
		}
		tableEnd += tableStart + len("</table>")
		tableChunk := pageHTML[tableStart:tableEnd]

		if strings.Contains(tableChunk, "wikitable") &&
			(strings.Contains(tableChunk, "Date established") || strings.Contains(tableChunk, "established as park")) {
			targetTable = tableChunk
			break
		}

		next := strings.Index(pageHTML[tableEnd:], "<table")
		if next == -1 {
			break
		}
		tableStart = tableEnd + next
	}

	if targetTable == "" {
		return nil, fmt.Errorf("national parks table not found in Wikipedia page")
	}

	rowRegex := regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	cellRegex := regexp.MustCompile(`(?is)<t[hd][^>]*>(.*?)</t[hd]>`)
	linkRegex := regexp.MustCompile(`(?is)<a[^>]+href="(/wiki/[^"]+)"[^>]*>(.*?)</a>`)
	imgRegex := regexp.MustCompile(`(?is)<img[^>]+src="([^"]+)"`)

	rows := rowRegex.FindAllStringSubmatch(targetTable, -1)
	var parks []Park

	for _, r := range rows {
		rowHTML := r[1]
		if !strings.Contains(rowHTML, "<td") {
			continue
		}

		cells := cellRegex.FindAllStringSubmatch(rowHTML, -1)
		if len(cells) < 6 {
			continue
		}

		nameCell := cells[0][1]
		nameLinks := linkRegex.FindStringSubmatch(nameCell)
		var name, wikiPath string
		if len(nameLinks) >= 3 {
			wikiPath = nameLinks[1]
			name = cleanText(nameLinks[2])
		} else {
			name = cleanText(nameCell)
		}

		isWHS := strings.Contains(nameCell, "*") || strings.Contains(nameCell, "‡")
		isBR := strings.Contains(nameCell, "†") || strings.Contains(nameCell, "‡")

		name = strings.Trim(name, "*†‡ \t\n\r")
		if name == "" || strings.EqualFold(name, "Name") {
			continue
		}

		fullWikiURL := ""
		if wikiPath != "" {
			fullWikiURL = "https://en.wikipedia.org" + wikiPath
		}

		imageCell := cells[1][1]
		imgMatch := imgRegex.FindStringSubmatch(imageCell)
		imageURL := ""
		if len(imgMatch) >= 2 {
			imageURL = imgMatch[1]
			if strings.HasPrefix(imageURL, "//") {
				imageURL = "https:" + imageURL
			}
		}

		locationCell := cells[2][1]
		rawLocation := cleanText(locationCell)
		state := rawLocation
		if idx := strings.IndexAny(rawLocation, "0123456789°"); idx != -1 {
			state = strings.TrimSpace(rawLocation[:idx])
		}

		dateEst := cleanText(cells[3][1])
		area := cleanText(cells[4][1])
		visitors := cleanText(cells[5][1])

		description := ""
		if len(cells) > 6 {
			description = cleanText(cells[6][1])
		}

		id := slugify(name)

		parks = append(parks, Park{
			ID:          id,
			Name:        name,
			State:       state,
			Location:    rawLocation,
			DateEst:     dateEst,
			Area:        area,
			Visitors:    visitors,
			Description: description,
			ImageURL:    imageURL,
			WikiURL:     fullWikiURL,
			IsWHS:       isWHS,
			IsBR:        isBR,
		})
	}

	return parks, nil
}

// ----------------- Seed Data (Fallback) -----------------
func getSeedParks() []Park {
	return []Park{
		{
			ID:          "acadia",
			Name:        "Acadia",
			State:       "Maine",
			Location:    "Maine 44°21′N 68°13′W",
			DateEst:     "February 26, 1919",
			Area:        "49,071.40 acres (198.6 km²)",
			Visitors:    "4,079,318",
			Description: "Covering most of Mount Desert Island and other coastal islands, Acadia features the tallest mountain on the Atlantic coast of the United States, granite peaks, ocean shoreline, woodlands, and lakes.",
			ImageURL:    "https://upload.wikimedia.org/wikipedia/commons/thumb/c/b2/Bass_Harbor_Head_Light_Station_2016.jpg/300px-Bass_Harbor_Head_Light_Station_2016.jpg",
			WikiURL:     "https://en.wikipedia.org/wiki/Acadia_National_Park",
		},
		{
			ID:          "yellowstone",
			Name:        "Yellowstone",
			State:       "Wyoming, Montana, Idaho",
			Location:    "Wyoming, Montana, Idaho 44°36′N 110°30′W",
			DateEst:     "March 1, 1872",
			Area:        "2,219,790.71 acres (8,983.2 km²)",
			Visitors:    "4,501,382",
			Description: "The first national park in the world, Yellowstone preserves the Grand Prismatic Spring, Old Faithful geyser, vast alpine rivers, canyons, and diverse wildlife including bison, grizzly bears, and wolves.",
			ImageURL:    "https://upload.wikimedia.org/wikipedia/commons/thumb/b/b5/Grand_Prismatic_Spring_2013.jpg/300px-Grand_Prismatic_Spring_2013.jpg",
			WikiURL:     "https://en.wikipedia.org/wiki/Yellowstone_National_Park",
			IsWHS:       true,
			IsBR:        true,
		},
		{
			ID:          "yosemite",
			Name:        "Yosemite",
			State:       "California",
			Location:    "California 37°50′N 119°30′W",
			DateEst:     "October 1, 1890",
			Area:        "761,747.50 acres (3,082.7 km²)",
			Visitors:    "3,897,070",
			Description: "Yosemite features towering granite cliffs such as El Capitan and Half Dome, dramatic waterfalls including Yosemite Falls, clear streams, giant sequoia groves, and biological diversity.",
			ImageURL:    "https://upload.wikimedia.org/wikipedia/commons/thumb/1/13/Tunnel_View%2C_Yosemite_Valley%2C_Yosemite_NP_-_Diliff.jpg/300px-Tunnel_View%2C_Yosemite_Valley%2C_Yosemite_NP_-_Diliff.jpg",
			WikiURL:     "https://en.wikipedia.org/wiki/Yosemite_National_Park",
			IsWHS:       true,
		},
	}
}

// ----------------- HTML Templates -----------------

const pageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>U.S. National Parks Explorer</title>
  <script src="https://unpkg.com/htmx.org@1.9.12"></script>
  <style>
    :root {
      --primary: #2d5a27;
      --primary-dark: #1e3d1a;
      --primary-light: #eaf3e8;
      --accent: #d97706;
      --bg: #f8fafc;
      --surface: #ffffff;
      --border: #e2e8f0;
      --text-main: #0f172a;
      --text-muted: #64748b;
      --font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: var(--font-family);
      background-color: var(--bg);
      color: var(--text-main);
      line-height: 1.5;
      height: 100vh;
      display: flex;
      flex-direction: column;
      overflow: hidden;
    }
    header {
      background: var(--primary);
      color: white;
      padding: 0.75rem 1.5rem;
      display: flex;
      align-items: center;
      justify-content: space-between;
      box-shadow: 0 2px 8px rgba(0,0,0,0.15);
      z-index: 10;
    }
    .header-left { display: flex; align-items: center; gap: 0.75rem; }
    .header-title { font-size: 1.25rem; font-weight: 700; letter-spacing: -0.025em; }
    .header-badge {
      font-size: 0.75rem;
      background: rgba(255, 255, 255, 0.2);
      padding: 0.2rem 0.6rem;
      border-radius: 9999px;
    }
    .btn {
      display: inline-flex;
      align-items: center;
      gap: 0.35rem;
      background: white;
      color: var(--primary);
      border: 1px solid rgba(255,255,255,0.3);
      padding: 0.45rem 0.9rem;
      border-radius: 0.375rem;
      font-size: 0.825rem;
      font-weight: 600;
      cursor: pointer;
      transition: all 0.15s ease;
      text-decoration: none;
    }
    .btn:hover { background: var(--primary-light); }
    .btn-secondary {
      background: var(--surface);
      color: var(--text-main);
      border: 1px solid var(--border);
    }
    .btn-secondary:hover { background: #f1f5f9; }
    .container {
      display: grid;
      grid-template-columns: 380px 1fr;
      flex: 1;
      height: calc(100vh - 60px);
      overflow: hidden;
    }
    @media (max-width: 840px) {
      .container { grid-template-columns: 1fr; }
      #detail-panel { display: none; }
      #detail-panel.active { display: block; position: fixed; inset: 0; z-index: 50; }
    }
    .sidebar {
      background: var(--surface);
      border-right: 1px solid var(--border);
      display: flex;
      flex-direction: column;
      height: 100%;
      overflow: hidden;
    }
    .search-box {
      padding: 1rem;
      border-bottom: 1px solid var(--border);
      background: #fafafa;
    }
    .search-input {
      width: 100%;
      padding: 0.6rem 0.9rem;
      border: 1px solid var(--border);
      border-radius: 0.5rem;
      font-size: 0.9rem;
      outline: none;
      transition: border-color 0.15s ease;
    }
    .search-input:focus {
      border-color: var(--primary);
      box-shadow: 0 0 0 3px rgba(45, 90, 39, 0.15);
    }
    .park-list {
      flex: 1;
      overflow-y: auto;
      list-style: none;
    }
    .park-item {
      display: flex;
      align-items: center;
      gap: 0.85rem;
      padding: 0.85rem 1rem;
      border-bottom: 1px solid var(--border);
      cursor: pointer;
      transition: background-color 0.15s ease;
    }
    .park-item:hover { background-color: var(--primary-light); }
    .park-item.selected {
      background-color: var(--primary-light);
      border-left: 4px solid var(--primary);
    }
    .park-thumb {
      width: 58px;
      height: 58px;
      border-radius: 0.375rem;
      object-fit: cover;
      background: #cbd5e1;
      flex-shrink: 0;
    }
    .park-info { flex: 1; min-width: 0; }
    .park-name {
      font-size: 0.95rem;
      font-weight: 600;
      color: var(--text-main);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .park-sub {
      font-size: 0.8rem;
      color: var(--text-muted);
      margin-top: 0.15rem;
    }
    .main-content {
      background: var(--bg);
      overflow-y: auto;
      height: 100%;
      padding: 2rem;
    }
    .card {
      background: var(--surface);
      border-radius: 0.75rem;
      border: 1px solid var(--border);
      box-shadow: 0 1px 3px rgba(0,0,0,0.05);
      overflow: hidden;
      max-width: 860px;
      margin: 0 auto;
    }
    .hero-img-wrap {
      width: 100%;
      max-height: 380px;
      overflow: hidden;
      background: #0f172a;
      position: relative;
    }
    .hero-img {
      width: 100%;
      height: 360px;
      object-fit: cover;
      display: block;
    }
    .card-body { padding: 1.75rem; }
    .badge-container { display: flex; gap: 0.5rem; flex-wrap: wrap; margin-bottom: 0.75rem; }
    .tag {
      font-size: 0.725rem;
      font-weight: 600;
      padding: 0.2rem 0.55rem;
      border-radius: 0.25rem;
      text-transform: uppercase;
      letter-spacing: 0.05em;
    }
    .tag-state { background: #f1f5f9; color: #475569; }
    .tag-whs { background: #dcfce7; color: #15803d; }
    .tag-br { background: #dbeafe; color: #1d4ed8; }
    .detail-title {
      font-size: 1.75rem;
      font-weight: 800;
      color: var(--text-main);
      margin-bottom: 0.5rem;
    }
    .stats-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
      gap: 1rem;
      margin: 1.5rem 0;
      background: #f8fafc;
      padding: 1.25rem;
      border-radius: 0.5rem;
      border: 1px solid var(--border);
    }
    .stat-label {
      font-size: 0.75rem;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      color: var(--text-muted);
    }
    .stat-value {
      font-size: 1rem;
      font-weight: 700;
      color: var(--text-main);
      margin-top: 0.2rem;
    }
    .description-text {
      font-size: 1rem;
      line-height: 1.7;
      color: #334155;
      margin-bottom: 1.75rem;
    }
    .actions { display: flex; gap: 0.75rem; flex-wrap: wrap; }
    .empty-state {
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      height: 70%;
      text-align: center;
      color: var(--text-muted);
    }
    .empty-state svg { width: 64px; height: 64px; margin-bottom: 1rem; stroke: #94a3b8; }
    .htmx-indicator { display: none; }
    .htmx-request .htmx-indicator { display: inline-block; }
    .spinner {
      border: 2px solid rgba(255,255,255,0.3);
      border-radius: 50%;
      border-top: 2px solid white;
      width: 14px;
      height: 14px;
      animation: spin 0.8s linear infinite;
    }
    @keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
  </style>
</head>
<body>

  <header>
    <div class="header-left">
      <span style="font-size:1.5rem;">🌲</span>
      <h1 class="header-title">U.S. National Parks</h1>
      <span class="header-badge" id="park-count">{{len .Parks}} Parks Loaded</span>
    </div>
    <div>
      <button class="btn"
              hx-post="/parks/refresh"
              hx-target="#app-view"
              hx-indicator="#refresh-spinner">
        <span class="spinner htmx-indicator" id="refresh-spinner"></span>
        <span>Sync Wikipedia</span>
      </button>
    </div>
  </header>

  <div id="app-view" class="container">
    {{template "app-content" .}}
  </div>

</body>
</html>`

const appContentTemplate = `{{define "app-content"}}
  <aside class="sidebar">
    <div class="search-box">
      <input type="search"
             class="search-input"
             placeholder="Search parks by name or state..."
             name="q"
             hx-get="/parks/search"
             hx-trigger="input changed delay:200ms, search"
             hx-target="#park-list"
             autocomplete="off">
    </div>

    <ul class="park-list" id="park-list">
      {{template "park-items" .}}
    </ul>
  </aside>

  <main class="main-content" id="detail-panel">
    {{if .SelectedPark}}
      {{template "park-detail" .SelectedPark}}
    {{else}}
      <div class="empty-state">
        <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <h3>Select a National Park</h3>
        <p>Choose any park from the list to view rich details, history, and statistics.</p>
      </div>
    {{end}}
  </main>
{{end}}`

const parkItemsTemplate = `{{define "park-items"}}
  {{range .Parks}}
    <li class="park-item {{if eq .ID $.SelectedID}}selected{{end}}"
        hx-get="/parks/detail?id={{.ID}}"
        hx-target="#detail-panel"
        hx-swap="innerHTML">
      {{if .ImageURL}}
        <img class="park-thumb" src="{{.ImageURL}}" alt="{{.Name}}" loading="lazy">
      {{else}}
        <div class="park-thumb" style="display:flex;align-items:center;justify-content:center;font-size:1.5rem;">🏞️</div>
      {{end}}
      <div class="park-info">
        <div class="park-name">{{.Name}}</div>
        <div class="park-sub">{{.State}}</div>
      </div>
    </li>
  {{else}}
    <li style="padding: 1.5rem; text-align: center; color: var(--text-muted); font-size: 0.9rem;">
      No parks matched your query.
    </li>
  {{end}}
{{end}}`

const parkDetailTemplate = `{{define "park-detail"}}
  <div class="card">
    {{if .ImageURL}}
      <div class="hero-img-wrap">
        <img class="hero-img" src="{{.ImageURL}}" alt="{{.Name}}">
      </div>
    {{end}}
    <div class="card-body">
      <div class="badge-container">
        {{if .State}}<span class="tag tag-state">{{.State}}</span>{{end}}
        {{if .IsWHS}}<span class="tag tag-whs">★ World Heritage Site</span>{{end}}
        {{if .IsBR}}<span class="tag tag-br">Biosphere Reserve</span>{{end}}
      </div>

      <h2 class="detail-title">{{.Name}} National Park</h2>

      <div class="stats-grid">
        <div>
          <div class="stat-label">Date Established</div>
          <div class="stat-value">{{if .DateEst}}{{.DateEst}}{{else}}N/A{{end}}</div>
        </div>
        <div>
          <div class="stat-label">Total Protected Area</div>
          <div class="stat-value">{{if .Area}}{{.Area}}{{else}}N/A{{end}}</div>
        </div>
        <div>
          <div class="stat-label">Annual Visitors</div>
          <div class="stat-value">{{if .Visitors}}{{.Visitors}}{{else}}N/A{{end}}</div>
        </div>
        <div>
          <div class="stat-label">Coordinates / Region</div>
          <div class="stat-value" style="font-size:0.85rem; font-weight:500;">
            {{if .Location}}{{.Location}}{{else}}N/A{{end}}
          </div>
        </div>
      </div>

      <div class="description-text">
        {{if .Description}}
          {{.Description}}
        {{else}}
          No description currently available from Wikipedia.
        {{end}}
      </div>

      <div class="actions">
        {{if .WikiURL}}
          <a href="{{.WikiURL}}" target="_blank" rel="noopener noreferrer" class="btn btn-secondary">
            Read on Wikipedia ↗
          </a>
        {{end}}
        <a href="https://www.google.com/maps/search/?api=1&query={{.Name}}+National+Park" target="_blank" rel="noopener noreferrer" class="btn btn-secondary">
          Open in Maps ↗
        </a>
      </div>
    </div>
  </div>
{{end}}`

type Server struct {
	store *ParkStore
	tmpl  *template.Template
}

func NewServer(store *ParkStore) (*Server, error) {
	t, err := template.New("base").Parse(pageTemplate)
	if err != nil {
		return nil, err
	}
	if _, err = t.Parse(appContentTemplate); err != nil {
		return nil, err
	}
	if _, err = t.Parse(parkItemsTemplate); err != nil {
		return nil, err
	}
	if _, err = t.Parse(parkDetailTemplate); err != nil {
		return nil, err
	}

	return &Server{
		store: store,
		tmpl:  t,
	}, nil
}

type PageData struct {
	Parks        []Park
	SelectedPark *Park
	SelectedID   string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	parks := s.store.GetAll()
	var selected *Park
	selectedID := r.URL.Query().Get("id")

	if selectedID != "" {
		if p, ok := s.store.GetByID(selectedID); ok {
			selected = &p
		}
	} else if len(parks) > 0 {
		selected = &parks[0]
		selectedID = parks[0].ID
	}

	data := PageData{
		Parks:        parks,
		SelectedPark: selected,
		SelectedID:   selectedID,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.Execute(w, data); err != nil {
		log.Printf("template execute error: %v", err)
	}
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	results := s.store.Search(query)

	data := PageData{
		Parks: results,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "park-items", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	park, ok := s.store.GetByID(id)
	if !ok {
		http.Error(w, "Park not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "park-detail", park); err != nil {
		log.Printf("template error: %v", err)
	}
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	log.Println("Manual sync triggered...")
	err := s.store.LoadFromWikipedia(wikiURL)
	if err != nil {
		log.Printf("Error refreshing from Wikipedia: %v", err)
	}

	parks := s.store.GetAll()
	var selected *Park
	var selectedID string
	if len(parks) > 0 {
		selected = &parks[0]
		selectedID = parks[0].ID
	}

	data := PageData{
		Parks:        parks,
		SelectedPark: selected,
		SelectedID:   selectedID,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "app-content", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	count, lastLoaded, err := s.store.Status()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"degraded","parks_count":%d,"error":%q}`, count, err.Error())
		return
	}
	fmt.Fprintf(w, `{"status":"ok","parks_count":%d,"last_loaded":%q}`, count, lastLoaded.Format(time.RFC3339))
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	store := NewParkStore()

	log.Printf("Downloading national parks data from: %s", wikiURL)
	if err := store.LoadFromWikipedia(wikiURL); err != nil {
		log.Printf("[WARNING] Could not fetch live Wikipedia data (%v). Loading fallback seed data...", err)
		seed := getSeedParks()
		store.Lock()
		store.parks = seed
		for _, p := range seed {
			store.parksByID[p.ID] = p
		}
		store.lastLoaded = time.Now()
		store.Unlock()
	} else {
		log.Printf("Successfully loaded %d national parks into memory!", len(store.GetAll()))
	}

	server, err := NewServer(store)
	if err != nil {
		log.Fatalf("failed to initialize templates: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", server.handleIndex)
	mux.HandleFunc("/parks/search", server.handleSearch)
	mux.HandleFunc("/parks/detail", server.handleDetail)
	mux.HandleFunc("/parks/refresh", server.handleRefresh)
	mux.HandleFunc("/healthz", server.handleHealth)

	httpServer := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server listening on port :%s", port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failure: %v", err)
		}
	}()

	<-stopChan
	log.Println("Shutting down server gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Server terminated.")
}
