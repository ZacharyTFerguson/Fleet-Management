const fs = require("node:fs");
const path = require("node:path");

const DEFAULT_FILE = path.join(__dirname, "..", "data", "important-locations.json");

function defaultCatalog() {
  return {
    version: 1,
    updatedAt: null,
    locations: [],
    stays: {},
  };
}

function readCatalog(filePath = DEFAULT_FILE) {
  try {
    const raw = fs.readFileSync(filePath, "utf8");
    const parsed = JSON.parse(raw);
    return {
      version: parsed.version ?? 1,
      updatedAt: parsed.updatedAt ?? null,
      locations: Array.isArray(parsed.locations) ? parsed.locations : [],
      stays: parsed.stays && typeof parsed.stays === "object" ? parsed.stays : {},
    };
  } catch (error) {
    if (error.code === "ENOENT") {
      return defaultCatalog();
    }
    throw error;
  }
}

function writeCatalog(catalog, filePath = DEFAULT_FILE) {
  const dir = path.dirname(filePath);
  fs.mkdirSync(dir, { recursive: true });
  const payload = {
    version: catalog.version ?? 1,
    updatedAt: catalog.updatedAt,
    locations: catalog.locations,
    stays: catalog.stays,
  };
  fs.writeFileSync(filePath, `${JSON.stringify(payload, null, 2)}\n`);
  return payload;
}

function saveState(state, filePath = DEFAULT_FILE) {
  return writeCatalog(
    {
      version: 1,
      updatedAt: new Date().toISOString(),
      locations: state.locations,
      stays: state.stays,
    },
    filePath,
  );
}

module.exports = {
  DEFAULT_FILE,
  defaultCatalog,
  readCatalog,
  writeCatalog,
  saveState,
};
