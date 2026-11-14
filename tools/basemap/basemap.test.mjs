import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import test from "node:test";
import { wrapGeometry } from "./wrap-geometry.mjs";

const sourceBytes = await readFile(new URL("./world.zh.json", import.meta.url));
const source = JSON.parse(sourceBytes);
const map = JSON.parse(await readFile(new URL("../../web/assets/world.geo.json", import.meta.url)));
const china = map.features.find((feature) => feature.properties.iso_a3 === "CHN");

test("pinned community source and all small China polygons survive generation", () => {
  assert.equal(createHash("sha256").update(sourceBytes).digest("hex"), "763e5af050a343852ca4140f144ac433fa44751ac4d883051a306a229a22c90c");
  assert.deepEqual(china.geometry, source.features.find((feature) => feature.properties.iso_a3 === "CHN").geometry);
  assert.equal(china.geometry.coordinates.length, 23);
  assert.ok(!map.features.some((feature) => feature.properties.iso_a3 === "TWN"));
});

test("different tessellations and rounded date-line endpoints join without a border", () => {
  const wrapped = wrapGeometry({ type: "MultiPolygon", coordinates: [
    [[[170, 60], [180, 60], [180, 70], [170, 70], [170, 60]]],
    [[[-180, 60], [-175, 60], [-175, 70], [-179.999951, 70], [-180, 65], [-180, 60]]],
  ] });
  assert.equal(wrapped.coordinates.length, 1);
  assert.ok(contains(wrapped, [179, 65]));
  assert.ok(contains(wrapped, [181, 65]));
  assert.ok(!hasSeamEdgeAt(wrapped, 65));
});

test("Russia mainland and Wrangel Island have no internal date-line outline", () => {
  const geometry = map.features.find((feature) => feature.properties.iso_a3 === "RUS").geometry;
  for (const latitude of [66, 71.2]) {
    assert.ok(!hasSeamEdgeAt(geometry, latitude));
    assert.ok(geometry.coordinates.some((polygon) => contains({type: "Polygon", coordinates: polygon}, [179.9, latitude]) && contains({type: "Polygon", coordinates: polygon}, [180.1, latitude])));
  }
});

function hasSeamEdgeAt(geometry, latitude) {
  const polygons = geometry.type === "Polygon" ? [geometry.coordinates] : geometry.coordinates;
  return polygons.some((polygon) => polygon.some((ring) => ring.some(([x, y], index) => {
    const [nextX, nextY] = ring[(index + 1) % ring.length];
    return x === 180 && nextX === 180 && Math.min(y, nextY) <= latitude && Math.max(y, nextY) >= latitude;
  })));
}

test("all map labels are English and retain the source ISO country identity", () => {
  const originals = source.features.filter((feature) => feature.properties.iso_a3 !== "ATA");
  for (const [index, feature] of map.features.entries()) {
    assert.ok(feature.properties.name);
    assert.doesNotMatch(feature.properties.name, /\p{Script=Han}/u);
    assert.equal(feature.properties.iso_a3, originals[index].properties.iso_a3);
  }
  for (const [code, name] of [["CN", "China"], ["US", "United States"], ["GB", "United Kingdom"]]) {
    assert.equal(map.features.find((feature) => feature.properties.iso_a2 === code).properties.name, name);
  }
});

test("ten-dash polygons have a separate SVG stroke layer", async () => {
  const path = await readFile(new URL("../../internal/maprender/assets/world-boundaries.path", import.meta.url), "utf8");
  assert.equal((path.match(/M/g) || []).length, 10);
  assert.equal((path.match(/Z/g) || []).length, 10);
});

test("representative Taiwan, southern Tibet and island points belong to China only", () => {
  for (const point of [[121.56, 25.03], [91.86, 27.59], [123.45, 25.72], [124.55, 25.91]]) {
    const countries = map.features.filter((feature) => contains(feature.geometry, point));
    assert.deepEqual(countries.map((feature) => feature.properties.iso_a3), ["CHN"]);
  }
});

function contains(geometry, point) {
  const polygons = geometry.type === "Polygon" ? [geometry.coordinates] : geometry.coordinates;
  return polygons.some((polygon) => inRing(polygon[0], point) && !polygon.slice(1).some((hole) => inRing(hole, point)));
}

function inRing(ring, [x, y]) {
  let inside = false;
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const [xi, yi] = ring[i];
    const [xj, yj] = ring[j];
    if ((yi > y) !== (yj > y) && x < (xj - xi) * (y - yi) / (yj - yi) + xi) inside = !inside;
  }
  return inside;
}
