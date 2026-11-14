import polygonClipping from "polygon-clipping";

// Use one longitude interval for fills and outlines. Union each country's
// pieces after wrapping, so source cuts at +/-180 cannot become borders.
export function wrapGeometry(geometry, left = -170) {
  const right = left + 360;
  const polygons = geometry.type === "Polygon" ? [geometry.coordinates] : geometry.coordinates;
  if (polygons.every((polygon) => polygon.every((ring) => ring.every(([x]) => x >= left && x <= right)))) {
    return geometry;
  }
  const viewport = [[[left, -90], [right, -90], [right, 90], [left, 90], [left, -90]]];
  const pieces = [];
  for (const polygon of polygons) {
    const rings = polygon.map((ring) => unwrap(ring, polygon[0][0][0]));
    const longitudes = rings[0].map(([x]) => x);
    const firstShift = Math.ceil((left - Math.max(...longitudes)) / 360);
    const lastShift = Math.floor((right - Math.min(...longitudes)) / 360);
    for (let shift = firstShift; shift <= lastShift; shift++) {
      const shifted = rings.map((ring) => ring.map(([x, y]) => [x + shift * 360, y]));
      pieces.push(...polygonClipping.intersection(shifted, viewport));
    }
  }
  const coordinates = pieces.length ? polygonClipping.union(...pieces) : [];
  return { type: "MultiPolygon", coordinates };
}

function unwrap(ring, anchor) {
  let previous = ring[0][0] + Math.round((anchor - ring[0][0]) / 360) * 360;
  return ring.map(([longitude, latitude]) => {
    // Source cuts can encode the same meridian as 180 or 179.999951.
    // Snap sub-0.0001-degree rounding gaps before joining their edges.
    if (Math.abs(Math.abs(longitude) - 180) < 0.0001) longitude = Math.sign(longitude) * 180;
    longitude += Math.round((previous - longitude) / 360) * 360;
    previous = longitude;
    return [longitude, latitude];
  });
}
