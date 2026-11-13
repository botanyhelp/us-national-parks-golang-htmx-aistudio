import express from 'express';
import fetch from 'node-fetch';

interface Park {
  id: string;
  name: string;
  state: string;
  location: string;
  date_established: string;
  area: string;
  visitors: string;
  description: string;
  image_url: string;
  wiki_url: string;
  is_whs: boolean;
  is_br: boolean;
}

class ParkStore {
  private parks: Park[] = [];
  private parksMap = new Map<string, Park>();
  private lastLoaded = new Date();
  private loadErr: string | null = null;

  constructor() {
    this.parks = this.getSeedParks();
    for (const p of this.parks) {
      this.parksMap.set(p.id, p);
    }
  }

  getSeedParks(): Park[] {
    return [
      {
        id: 'acadia',
        name: 'Acadia',
        state: 'Maine',
        location: 'Maine 44°21′N 68°13′W',
        date_established: 'February 26, 1919',
        area: '49,071.40 acres (198.6 km²)',
        visitors: '4,079,318',
        description: 'Covering most of Mount Desert Island and other coastal islands, Acadia features the tallest mountain on the Atlantic coast of the United States, granite peaks, ocean shoreline, woodlands, and lakes.',
        image_url: 'https://upload.wikimedia.org/wikipedia/commons/thumb/c/b2/Bass_Harbor_Head_Light_Station_2016.jpg/300px-Bass_Harbor_Head_Light_Station_2016.jpg',
        wiki_url: 'https://en.wikipedia.org/wiki/Acadia_National_Park',
        is_whs: false,
        is_br: false,
      },
      {
        id: 'yellowstone',
        name: 'Yellowstone',
        state: 'Wyoming, Montana, Idaho',
        location: 'Wyoming, Montana, Idaho 44°36′N 110°30′W',
        date_established: 'March 1, 1872',
        area: '2,219,790.71 acres (8,983.2 km²)',
        visitors: '4,501,382',
        description: 'The first national park in the world, Yellowstone preserves the Grand Prismatic Spring, Old Faithful geyser, vast alpine rivers, canyons, and diverse wildlife including bison, grizzly bears, and wolves.',
        image_url: 'https://upload.wikimedia.org/wikipedia/commons/thumb/b/b5/Grand_Prismatic_Spring_2013.jpg/300px-Grand_Prismatic_Spring_2013.jpg',
        wiki_url: 'https://en.wikipedia.org/wiki/Yellowstone_National_Park',
        is_whs: true,
        is_br: true,
      },
      {
        id: 'yosemite',
        name: 'Yosemite',
        state: 'California',
        location: 'California 37°50′N 119°30′W',
        date_established: 'October 1, 1890',
        area: '761,747.50 acres (3,082.7 km²)',
        visitors: '3,897,070',
        description: 'Yosemite features towering granite cliffs such as El Capitan and Half Dome, dramatic waterfalls including Yosemite Falls, clear streams, giant sequoia groves, and biological diversity.',
        image_url: 'https://upload.wikimedia.org/wikipedia/commons/thumb/1/13/Tunnel_View%2C_Yosemite_Valley%2C_Yosemite_NP_-_Diliff.jpg/300px-Tunnel_View%2C_Yosemite_Valley%2C_Yosemite_NP_-_Diliff.jpg',
        wiki_url: 'https://en.wikipedia.org/wiki/Yosemite_National_Park',
        is_whs: true,
        is_br: false,
      },
    ];
  }

  async loadFromWikipedia(): Promise<void> {
    const url = 'https://en.wikipedia.org/wiki/List_of_national_parks_of_the_United_States';
    try {
      const resp = await fetch(url, {
        headers: {
          'User-Agent': 'USNationalParksViewer/1.0 (NodeJS HTMX Demo; contact@example.com)',
          'Accept': 'text/html',
        },
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const htmlText = await resp.text();
      const parsed = this.parseHTML(htmlText);
      if (parsed.length > 0) {
        this.parks = parsed;
        this.parksMap.clear();
        for (const p of this.parks) {
          this.parksMap.set(p.id, p);
        }
        this.lastLoaded = new Date();
        this.loadErr = null;
      }
    } catch (err: any) {
      console.error('Failed to fetch Wikipedia live:', err);
      this.loadErr = err.message;
    }
  }

  private parseHTML(html: string): Park[] {
    const tableMatch = html.match(/<table[^>]*class="[^"]*wikitable[^"]*"[^>]*>([\s\S]*?)<\/table>/i);
    if (!tableMatch) return this.parks;

    const rowRegex = /<tr[^>]*>([\s\S]*?)<\/tr>/gi;
    const rows = [...tableMatch[1].matchAll(rowRegex)];
    const results: Park[] = [];

    for (const r of rows) {
      const rowHTML = r[1];
      if (!rowHTML.includes('<td')) continue;

      const cellRegex = /<t[hd][^>]*>([\s\S]*?)<\/t[hd]>/gi;
      const cells = [...rowHTML.matchAll(cellRegex)].map(c => c[1]);
      if (cells.length < 6) continue;

      const nameCell = cells[0];
      const linkMatch = nameCell.match(/<a[^>]+href="(\/wiki\/[^"]+)"[^>]*>([\s\S]*?)<\/a>/i);
      let name = '';
      let wikiPath = '';
      if (linkMatch) {
        wikiPath = linkMatch[1];
        name = this.clean(linkMatch[2]);
      } else {
        name = this.clean(nameCell);
      }

      const isWHS = nameCell.includes('*') || nameCell.includes('‡');
      const isBR = nameCell.includes('†') || nameCell.includes('‡');
      name = name.replace(/[*†‡]/g, '').trim();

      if (!name || name.toLowerCase() === 'name') continue;

      const wikiUrl = wikiPath ? `https://en.wikipedia.org${wikiPath}` : '';
      const imgMatch = cells[1].match(/<img[^>]+src="([^"]+)"/i);
      let imageUrl = imgMatch ? imgMatch[1] : '';
      if (imageUrl.startsWith('//')) imageUrl = 'https:' + imageUrl;

      const rawLoc = this.clean(cells[2]);
      let state = rawLoc;
      const idx = state.search(/[0-9°]/);
      if (idx !== -1) state = state.substring(0, idx).trim();

      const dateEst = this.clean(cells[3]);
      const area = this.clean(cells[4]);
      const visitors = this.clean(cells[5]);
      const description = cells.length > 6 ? this.clean(cells[6]) : '';
      const id = this.slugify(name);

      results.push({
        id,
        name,
        state,
        location: rawLoc,
        date_established: dateEst,
        area,
        visitors,
        description,
        image_url: imageUrl,
        wiki_url: wikiUrl,
        is_whs: isWHS,
        is_br: isBR,
      });
    }

    return results.length > 0 ? results : this.parks;
  }

  private clean(str: string): string {
    return str
      .replace(/\[\d+\]/g, '')
      .replace(/<[^>]*>/g, ' ')
      .replace(/&amp;/g, '&')
      .replace(/&#160;/g, ' ')
      .replace(/\s+/g, ' ')
      .trim();
  }

  private slugify(str: string): string {
    return str
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '');
  }

  getAll(): Park[] {
    return this.parks;
  }

  getByID(id: string): Park | undefined {
    return this.parksMap.get(id);
  }

  search(q: string): Park[] {
    const query = q.toLowerCase().trim();
    if (!query) return this.parks;
    return this.parks.filter(
      p =>
        p.name.toLowerCase().includes(query) ||
        p.state.toLowerCase().includes(query) ||
        p.description.toLowerCase().includes(query)
    );
  }

  status() {
    return { count: this.parks.length, lastLoaded: this.lastLoaded, error: this.loadErr };
  }
}

const store = new ParkStore();
const app = express();
const PORT = process.env.PORT || 3000;

app.use(express.urlencoded({ extended: true }));
app.use(express.json());

function renderParkItems(parks: Park[], selectedID: string): string {
  if (parks.length === 0) {
    return `<li style="padding: 1.5rem; text-align: center; color: var(--text-muted); font-size: 0.9rem;">No parks matched your query.</li>`;
  }
  return parks
    .map(
      p => `
    <li class="park-item ${p.id === selectedID ? 'selected' : ''}"
        hx-get="/?id=${p.id}"
        hx-target="#app-view"
        hx-select="#app-view"
        hx-push-url="true"
        hx-swap="innerHTML"
        hx-on::after-request="document.getElementById('detail-panel').classList.add('active')">
      ${
        p.image_url
          ? `<img class="park-thumb" src="${p.image_url}" alt="${p.name}" loading="lazy">`
          : `<div class="park-thumb" style="display:flex;align-items:center;justify-content:center;font-size:1.5rem;">🏞️</div>`
      }
      <div class="park-info">
        <div class="park-name">${p.name}</div>
        <div class="park-sub">${p.state}</div>
      </div>
    </li>
  `
    )
    .join('');
}

function renderParkDetail(p: Park): string {
  return `
  <div class="card">
    <button class="btn btn-secondary mobile-back-btn" onclick="document.getElementById('detail-panel').classList.remove('active')">
      ← Back to Parks
    </button>
    ${
      p.image_url
        ? `<div class="hero-img-wrap"><img class="hero-img" src="${p.image_url}" alt="${p.name}"></div>`
        : ''
    }
    <div class="card-body">
      <div class="badge-container">
        ${p.state ? `<span class="tag tag-state">${p.state}</span>` : ''}
        ${p.is_whs ? `<span class="tag tag-whs">★ World Heritage Site</span>` : ''}
        ${p.is_br ? `<span class="tag tag-br">Biosphere Reserve</span>` : ''}
      </div>

      <h2 class="detail-title">${p.name} National Park</h2>

      <div class="stats-grid">
        <div>
          <div class="stat-label">Date Established</div>
          <div class="stat-value">${p.date_established || 'N/A'}</div>
        </div>
        <div>
          <div class="stat-label">Total Protected Area</div>
          <div class="stat-value">${p.area || 'N/A'}</div>
        </div>
        <div>
          <div class="stat-label">Annual Visitors</div>
          <div class="stat-value">${p.visitors || 'N/A'}</div>
        </div>
        <div>
          <div class="stat-label">Coordinates / Region</div>
          <div class="stat-value" style="font-size:0.85rem; font-weight:500;">${p.location || 'N/A'}</div>
        </div>
      </div>

      <div class="description-text">
        ${p.description || 'No description currently available from Wikipedia.'}
      </div>

      <div class="actions">
        ${
          p.wiki_url
            ? `<a href="${p.wiki_url}" target="_blank" rel="noopener noreferrer" class="btn btn-secondary">Read on Wikipedia ↗</a>`
            : ''
        }
        <a href="https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(
          p.name + ' National Park'
        )}" target="_blank" rel="noopener noreferrer" class="btn btn-secondary">Open in Maps ↗</a>
      </div>
    </div>
  </div>
  `;
}

function renderAppContent(parks: Park[], selectedPark: Park | undefined, selectedID: string): string {
  return `
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
      ${renderParkItems(parks, selectedID)}
    </ul>
  </aside>

  <main class="main-content ${selectedPark ? 'active' : ''}" id="detail-panel">
    ${selectedPark ? renderParkDetail(selectedPark) : `
      <div class="empty-state">
        <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <h3>Select a National Park</h3>
        <p>Choose any park from the list to view rich details, history, and statistics.</p>
      </div>
    `}
  </main>
  `;
}

function renderPage(parks: Park[], selectedPark: Park | undefined, selectedID: string): string {
  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>U.S. National Parks Explorer (Go & htmx)</title>
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
      position: relative;
    }
    .mobile-back-btn { display: none; }
    @media (max-width: 840px) {
      .container { grid-template-columns: 1fr; }
      #detail-panel {
        display: none;
        position: absolute;
        inset: 0;
        z-index: 50;
        background: var(--bg);
      }
      #detail-panel.active { display: block; }
      .mobile-back-btn { display: inline-flex; margin-bottom: 1rem; }
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
    .htmx-request.htmx-indicator { display: inline-block; }
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
      <span class="header-badge" id="park-count">${parks.length} Parks Loaded</span>
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
    ${renderAppContent(parks, selectedPark, selectedID)}
  </div>

</body>
</html>`;
}

// Routes
app.get('/', (req, res) => {
  const parks = store.getAll();
  const selectedID = (req.query.id as string) || (parks.length > 0 ? parks[0].id : '');
  const selectedPark = store.getByID(selectedID) || parks[0];
  
  if (req.headers['hx-request']) {
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.send(renderAppContent(parks, selectedPark, selectedID));
  } else {
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.send(renderPage(parks, selectedPark, selectedID));
  }
});

app.get('/parks/search', (req, res) => {
  const q = (req.query.q as string) || '';
  const results = store.search(q);
  const selectedID = (req.query.id as string) || '';
  res.setHeader('Content-Type', 'text/html; charset=utf-8');
  res.send(renderParkItems(results, selectedID));
});

app.get('/parks/detail', (req, res) => {
  const id = (req.query.id as string) || '';
  const park = store.getByID(id);
  if (!park) {
    res.status(404).send('Park not found');
    return;
  }
  res.setHeader('Content-Type', 'text/html; charset=utf-8');
  res.send(renderParkDetail(park));
});

app.post('/parks/refresh', async (req, res) => {
  await store.loadFromWikipedia();
  const parks = store.getAll();
  const selectedPark = parks.length > 0 ? parks[0] : undefined;
  const selectedID = selectedPark ? selectedPark.id : '';
  res.setHeader('Content-Type', 'text/html; charset=utf-8');
  res.send(renderAppContent(parks, selectedPark, selectedID));
});

app.get('/healthz', (req, res) => {
  const { count, lastLoaded, error } = store.status();
  res.json({ status: error ? 'degraded' : 'ok', parks_count: count, last_loaded: lastLoaded, error });
});

// Initial load in background
store.loadFromWikipedia().then(() => {
  console.log(`Loaded ${store.getAll().length} parks.`);
});

app.listen(Number(PORT), '0.0.0.0', () => {
  console.log(`Node/htmx development server running on http://0.0.0.0:${PORT}`);
});
