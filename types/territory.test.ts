/**
 * Compile-and-run proof that types/territory.d.ts describes the JSON that the
 * v2 emitter actually ships. The fixtures in testdata/ are verbatim files from
 * the published data release (data-2026-11-15), not hand-written approximations.
 *
 * No package.json and no @types/node are needed: the two ambient declarations
 * below are the only Node surface this file uses.
 *
 *   npx --yes --package typescript@5.9.3 tsc --strict --target es2020 \
 *       --module commonjs --moduleResolution node --noEmit types/territory.test.ts
 *   npx --yes --package typescript@5.9.3 tsc --strict --target es2020 \
 *       --module commonjs --moduleResolution node --outDir /tmp/opencode/ts-test types/territory.test.ts
 *   node /tmp/opencode/ts-test/territory.test.js   # run from the repository root
 */

import type {
  BBox,
  Feature,
  FeatureCollection,
  IndexEntry,
  Manifest,
  MultiPolygon,
  Polygon,
  Position,
} from "./territory";

declare const require: (id: string) => any;
declare const process: { cwd(): string };
declare const console: { log(...args: unknown[]): void };

const fs = require("fs") as { readFileSync(file: string, encoding: string): string };
const nodePath = require("path") as { join(...parts: string[]): string };
const testdata = nodePath.join(process.cwd(), "types", "testdata");

function fixture(name: string): unknown {
  return JSON.parse(fs.readFileSync(nodePath.join(testdata, name), "utf8"));
}

let checks = 0;
function check(condition: unknown, message: string): void {
  checks++;
  if (!condition) throw new Error(message);
}

// ---------------------------------------------------------------------------
// Compile-time contract checks. Each @ts-expect-error must find a real error:
// if the declaration file ever loosens, tsc fails on the unused directive.
// ---------------------------------------------------------------------------
// @ts-expect-error osm_id is required on every properties object
const missingOsmId: IndexEntry = { level: "province", admin_level: 4, name: "x", bbox: [0, 0, 1, 1], center: [0, 0] };
// @ts-expect-error a Feature id is the numeric osm_id, never a string
const stringId: Feature = { type: "Feature", id: "1", bbox: [0, 0, 1, 1], properties: { osm_id: 1, level: "village", admin_level: 7, name: "" }, geometry: { type: "Polygon", coordinates: [] } };
// @ts-expect-error a Position has exactly two numbers, [lon, lat]
const longPosition: Position = [1, 2, 3];
// @ts-expect-error bbox has exactly four numbers
const shortBBox: BBox = [1, 2, 3];
// @ts-expect-error the shipped key is osm_id, never the camelCase osmId
const camelCaseKey: IndexEntry = { osm_id: 1, osmId: 1, level: "village", admin_level: 7, name: "", bbox: [0, 0, 1, 1], center: [0, 0] };

/** Compiles only while geometry is a true discriminated union. */
function ringCount(geometry: Polygon | MultiPolygon): number {
  switch (geometry.type) {
    case "Polygon":
      return geometry.coordinates.length;
    case "MultiPolygon":
      return geometry.coordinates.reduce((sum, polygon) => sum + polygon.length, 0);
  }
}

// Keep the compile-time-only values referenced without pretending to test them.
void missingOsmId;
void stringId;
void longPosition;
void shortBBox;
void camelCaseKey;

// ---------------------------------------------------------------------------
// Real index entries: numeric ids, required fields, absent/present optionals.
// ---------------------------------------------------------------------------
const provinces = fixture("index-provinces.json") as IndexEntry[];
check(Array.isArray(provinces) && provinces.length > 0, "provinces fixture is not a non-empty array");
const jabar = provinces[0];
check(Number.isInteger(jabar.osm_id), "osm_id must be a JSON number, not a string");
check(jabar.level === "province" && jabar.admin_level === 4, "required level fields disagree with the fixture");
check(jabar.name.length > 0, "expected a non-empty province name here");
check(jabar.bbox.length === 4 && jabar.center.length === 2, "bbox/center arity");
check(jabar.parent_id === undefined && jabar.ancestors === undefined, "provinces have no ancestor");
check(jabar.children !== undefined, "provinces carry child counts");
for (const entry of provinces) {
  check(Number.isInteger(entry.osm_id), `non-numeric osm_id: ${String(entry.osm_id)}`);
  check(typeof entry.name === "string", "name is always a string, even when empty");
}

const withBlanks = fixture("index-empty-name.json") as IndexEntry[];
const blank = withBlanks.find((entry) => entry.name === "");
check(blank !== undefined, "no empty-name entry in the real district fixture");
if (blank !== undefined) {
  check(Number.isInteger(blank.osm_id) && blank.osm_id > 1_000_000, "empty-name region has a large numeric id");
  check(blank.name === "", "empty name must survive decoding as an empty string");
  check(typeof blank.wikidata === "string" && blank.children !== undefined, "optional wikidata/children are present here");
  check(blank.code === undefined, "absent optional code stays undefined");
}

// ---------------------------------------------------------------------------
// Real features: both geometry kinds, integer coordinates, optional properties.
// ---------------------------------------------------------------------------
const polygonCollection = fixture("polygon.geojson") as FeatureCollection;
check(polygonCollection.type === "FeatureCollection", "wrong collection type");
const polygonFeature = polygonCollection.features[0];
check(polygonFeature.type === "Feature", "wrong feature type");
check(Number.isInteger(polygonFeature.id) && polygonFeature.id === polygonFeature.properties.osm_id, "id must equal osm_id");
check(polygonFeature.geometry.type === "Polygon", "expected a Polygon fixture");
if (polygonFeature.geometry.type === "Polygon") {
  const outer = polygonFeature.geometry.coordinates[0];
  check(outer.length >= 4, "a ring must be closed with at least four positions");
  check(ringCount(polygonFeature.geometry) === polygonFeature.geometry.coordinates.length, "ringCount on Polygon");
  const whole = outer.find((point) => Number.isInteger(point[0]) || Number.isInteger(point[1]));
  check(whole !== undefined, "expected a whole-number coordinate in the real fixture");
  if (whole !== undefined) {
    check(typeof whole[0] === "number" && typeof whole[1] === "number", "integer coordinates are still numbers");
  }
}
check(polygonFeature.properties.kind === undefined && polygonFeature.properties.wikidata === undefined, "absent optionals stay undefined");

const multiCollection = fixture("multipolygon.geojson") as FeatureCollection;
const multiFeature = multiCollection.features[0];
check(multiFeature.geometry.type === "MultiPolygon", "expected a MultiPolygon fixture");
if (multiFeature.geometry.type === "MultiPolygon") {
  check(multiFeature.geometry.coordinates.length > 1, "expected an island MultiPolygon");
  check(ringCount(multiFeature.geometry) > 0, "ringCount on MultiPolygon");
}

const mixed = fixture("mixed.geojson") as FeatureCollection;
const kinds = mixed.features.map((feature) => feature.geometry.type);
check(kinds.includes("Polygon") && kinds.includes("MultiPolygon"), "mixed fixture should contain both geometry kinds");
for (const feature of mixed.features) {
  check(ringCount(feature.geometry) > 0, "every feature must expose at least one ring");
}

// ---------------------------------------------------------------------------
// Real manifest: required values, optional provenance fields, tier metadata.
// ---------------------------------------------------------------------------
const manifest = fixture("manifest.json") as Manifest;
check(manifest.schema_version === 2, "schema_version must be the literal 2");
check(manifest.source.osm_timestamp !== undefined, "published manifest records an osm_timestamp");
check(manifest.source.extracts !== undefined && manifest.source.extracts.length > 0, "published manifest lists extracts");
check(manifest.counts.provinces > 0 && manifest.counts.villages > 0, "counts are numeric");
check(manifest.tiers["index"]?.dir === "index", "tiers are keyed by tier name");
check(manifest.license === "ODbL-1.0", "license is the exact schema constant");

console.log(`territory.test.ts: ${checks} checks passed against real v2 fixtures`);
