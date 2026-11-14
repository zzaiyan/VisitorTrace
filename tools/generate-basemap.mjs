import { readFile, writeFile } from "node:fs/promises";
import { wrapGeometry } from "./basemap/wrap-geometry.mjs";

const [inputPath, outputPath, geoJSONOutputPath] = process.argv.slice(2);
if (!inputPath || !outputPath) {
  throw new Error("usage: node tools/generate-basemap.mjs <countries.geojson> <world.path> [world.geo.json]");
}

const source = JSON.parse(await readFile(inputPath, "utf8"));
const paths = [];
const linearPaths = [];
const layers = JSON.parse(await readFile(new URL("./basemap/layers.json", import.meta.url), "utf8"));
const minLatitude = -60;
const maxLatitude = 90;
const mapWidth = 1000;
const mapHeight = mapWidth * (maxLatitude - minLatitude) / 360;
const countryNames = new Intl.DisplayNames(["en"], { type: "region", fallback: "none" });
const regionNames = JSON.parse(await readFile(new URL("./basemap/region-names.en.json", import.meta.url), "utf8"));
const features = source.features
  .filter((feature) => feature.properties.iso_a3 !== "ATA")
  .map((feature) => {
    const { iso_a2, iso_a3, iso_n3, full_name } = feature.properties;
    const name = /^[A-Z]{2}$/.test(iso_a2) ? countryNames.of(iso_a2) : regionNames[full_name];
    if (!name) throw new Error(`Missing English map label: ${full_name}`);
    return { ...feature, properties: { name, iso_a2, iso_a3, iso_n3 }, geometry: wrapGeometry(feature.geometry) };
  });

for (const feature of features) {
  const geometry = feature.geometry;
  const polygons = geometry.type === "Polygon" ? [geometry.coordinates] : geometry.coordinates;
  for (const [polygonIndex, polygon] of polygons.entries()) {
    for (const ring of polygon) {
      // Keep every vertex, including small islands and the ten-dash polygons.
      // Unwrap the date line before projecting so a ring cannot cross the map.
      let longitude = ring[0][0];
      const points = ring.map(([nextLongitude, latitude]) => {
        while (nextLongitude - longitude > 180) nextLongitude -= 360;
        while (nextLongitude - longitude < -180) nextLongitude += 360;
        longitude = nextLongitude;
        return project([longitude, latitude]);
      });
      const path = "M" + points.map(([x, y]) => `${format(x)} ${format(y)}`).join("L") + "Z";
      paths.push(path);
      if (layers.linearPolygons[feature.properties.iso_a3]?.includes(polygonIndex)) linearPaths.push(path);
    }
  }
}

await writeFile(outputPath, paths.join(""), "utf8");
await writeFile(outputPath.replace(/\.path$/, "-boundaries.path"), linearPaths.join(""), "utf8");
if (geoJSONOutputPath) {
  // SVG and ECharts share the same wrapped, joined country geometry.
  await writeFile(geoJSONOutputPath, JSON.stringify({ type: "FeatureCollection", features }), "utf8");
}

function project([longitude, latitude]) {
  const visibleLatitude = Math.max(minLatitude, Math.min(maxLatitude, latitude));
  return [
    ((longitude + 180) / 360) * mapWidth,
    ((maxLatitude - visibleLatitude) / (maxLatitude - minLatitude)) * mapHeight,
  ];
}

function format(value) {
  return Number(value.toFixed(6)).toString();
}
