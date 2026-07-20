import { readFile, writeFile } from "node:fs/promises";

const [inputPath, outputPath] = process.argv.slice(2);
if (!inputPath || !outputPath) {
  throw new Error("usage: node tools/generate-basemap.mjs <countries.geojson> <world.path>");
}

const source = JSON.parse(await readFile(inputPath, "utf8"));
const paths = [];
const minLatitude = -60;
const maxLatitude = 90;
const mapWidth = 1000;
const mapHeight = mapWidth * (maxLatitude - minLatitude) / 360;

for (const feature of source.features || []) {
  if (isAntarctica(feature.properties || {})) continue;
  const geometry = feature.geometry;
  if (!geometry) continue;
  const polygons = geometry.type === "Polygon"
    ? [geometry.coordinates]
    : geometry.type === "MultiPolygon"
      ? geometry.coordinates
      : [];
  for (const polygon of polygons) {
    for (const ring of polygon) {
      const points = ring.map(project);
      const simplified = simplifyClosed(points, 0.8);
      if (simplified.length < 3) continue;
      paths.push(
        "M" + simplified.map(([x, y]) => `${format(x)} ${format(y)}`).join("L") + "Z"
      );
    }
  }
}

await writeFile(outputPath, paths.join(""), "utf8");

function project([longitude, latitude]) {
  return [
    ((longitude + 180) / 360) * mapWidth,
    ((maxLatitude - latitude) / (maxLatitude - minLatitude)) * mapHeight,
  ];
}

function isAntarctica(properties) {
  return properties.ADM0_A3 === "ATA"
    || properties.ISO_A3 === "ATA"
    || properties.ADMIN === "Antarctica"
    || properties.NAME === "Antarctica";
}

function simplifyClosed(points, tolerance) {
  if (points.length > 1 && equal(points[0], points[points.length - 1])) {
    points = points.slice(0, -1);
  }
  return simplify(points, tolerance * tolerance);
}

function simplify(points, squareTolerance) {
  if (points.length <= 2) return points;
  let maxDistance = 0;
  let index = 0;
  const first = points[0];
  const last = points[points.length - 1];
  for (let i = 1; i < points.length - 1; i += 1) {
    const distance = squareSegmentDistance(points[i], first, last);
    if (distance > maxDistance) {
      index = i;
      maxDistance = distance;
    }
  }
  if (maxDistance <= squareTolerance) return [first, last];
  const left = simplify(points.slice(0, index + 1), squareTolerance);
  const right = simplify(points.slice(index), squareTolerance);
  return left.slice(0, -1).concat(right);
}

function squareSegmentDistance(point, start, end) {
  let x = start[0];
  let y = start[1];
  let dx = end[0] - x;
  let dy = end[1] - y;
  if (dx !== 0 || dy !== 0) {
    const t = ((point[0] - x) * dx + (point[1] - y) * dy) / (dx * dx + dy * dy);
    if (t > 1) {
      x = end[0];
      y = end[1];
    } else if (t > 0) {
      x += dx * t;
      y += dy * t;
    }
  }
  dx = point[0] - x;
  dy = point[1] - y;
  return dx * dx + dy * dy;
}

function equal(left, right) {
  return left[0] === right[0] && left[1] === right[1];
}

function format(value) {
  return Number(value.toFixed(1)).toString();
}
