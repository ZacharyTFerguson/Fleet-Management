const http = require("node:http");
const fs = require("node:fs");
const path = require("node:path");
const { ingestPing } = require("./lib/ingest");
const { readCatalog } = require("./lib/store");
const { DWELL_MINUTES, DWELL_RADIUS_METERS } = require("./lib/defaults");

const PORT = Number(process.env.PORT) || 3000;
const DATA_FILE = process.env.IMPORTANT_LOCATIONS_FILE
  ? path.resolve(process.env.IMPORTANT_LOCATIONS_FILE)
  : path.join(__dirname, "data", "important-locations.json");
const PUBLIC_DIR = path.join(__dirname, "public");

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml",
};

function sendJson(res, status, body) {
  const payload = JSON.stringify(body);
  res.writeHead(status, {
    "Content-Type": "application/json; charset=utf-8",
    "Cache-Control": "no-store",
  });
  res.end(payload);
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
    req.on("error", reject);
  });
}

function catalogView() {
  const catalog = readCatalog(DATA_FILE);
  const locations = [...catalog.locations].sort((a, b) =>
    String(b.lastSeenAt).localeCompare(String(a.lastSeenAt)),
  );
  return {
    dwellMinutes: DWELL_MINUTES,
    dwellRadiusMeters: DWELL_RADIUS_METERS,
    updatedAt: catalog.updatedAt,
    count: locations.length,
    locations,
  };
}

function serveStatic(req, res) {
  const url = new URL(req.url, "http://localhost");
  let filePath = url.pathname === "/" ? "/index.html" : url.pathname;
  filePath = path.normalize(filePath).replace(/^(\.\.[/\\])+/, "");
  const abs = path.join(PUBLIC_DIR, filePath);
  if (!abs.startsWith(PUBLIC_DIR)) {
    res.writeHead(403);
    res.end("Forbidden");
    return;
  }
  fs.readFile(abs, (error, data) => {
    if (error) {
      res.writeHead(error.code === "ENOENT" ? 404 : 500);
      res.end(error.code === "ENOENT" ? "Not found" : "Error");
      return;
    }
    res.writeHead(200, { "Content-Type": MIME[path.extname(abs)] || "application/octet-stream" });
    res.end(data);
  });
}

async function handle(req, res) {
  const url = new URL(req.url, "http://localhost");

  if (req.method === "GET" && url.pathname === "/api/v1/health") {
    sendJson(res, 200, { ok: true, service: "important-locations" });
    return;
  }

  if (req.method === "GET" && url.pathname === "/api/v1/important-locations") {
    const view = catalogView();
    const vehicle = url.searchParams.get("vehicle");
    if (vehicle) {
      view.locations = view.locations.filter(
        (location) => location.vehicleId.toLowerCase() === vehicle.toLowerCase(),
      );
      view.count = view.locations.length;
    }
    sendJson(res, 200, view);
    return;
  }

  if (req.method === "POST" && url.pathname === "/api/v1/pings") {
    const rawText = await readBody(req);
    let payload = {};
    if (rawText.trim()) {
      try {
        payload = JSON.parse(rawText);
      } catch {
        sendJson(res, 400, { recorded: false, reason: "invalid_json" });
        return;
      }
    }
    const result = ingestPing(payload, { filePath: DATA_FILE });
    const status = result.reason === "invalid_json" ? 400 : 200;
    sendJson(res, status, {
      recorded: result.recorded,
      reason: result.reason,
      vehicleId: result.vehicleId,
      location: result.location,
      stay: result.stay
        ? {
            vehicleId: result.stay.vehicleId,
            lat: result.stay.lat,
            lng: result.stay.lng,
            arrivedAt: result.stay.arrivedAt,
            lastSeenAt: result.stay.lastSeenAt,
          }
        : null,
    });
    return;
  }

  if (req.method === "GET" || req.method === "HEAD") {
    serveStatic(req, res);
    return;
  }

  sendJson(res, 404, { error: "not_found" });
}

function createServer() {
  return http.createServer((req, res) => {
    handle(req, res).catch((error) => {
      sendJson(res, 500, { error: "internal_error", message: error.message });
    });
  });
}

if (require.main === module) {
  const server = createServer();
  server.listen(PORT, () => {
    process.stdout.write(`Important locations listening on http://127.0.0.1:${PORT}\n`);
  });
}

module.exports = { createServer, DATA_FILE };
