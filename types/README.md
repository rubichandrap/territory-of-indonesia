# Dataset model types

TypeScript and Dart models for the **v2** dataset. Both follow
[`schema/territory.schema.json`](../schema/territory.schema.json): snake_case
JSON keys, numeric ids, and a `Polygon | MultiPolygon` geometry union.

## TypeScript — `territory.d.ts`

Import the declaration file and type the result of `fetch`:

```ts
import type { FeatureCollection, IndexEntry, Manifest } from "./territory";

const provinces: IndexEntry[] = await (
  await fetch(`${base}/index/provinces.json`)
).json();

const features: FeatureCollection = await (
  await fetch(`${base}/simplified/villages/${id}.geojson`)
).json();

for (const feature of features.features) {
  if (feature.geometry.type === "Polygon") {
    const rings = feature.geometry.coordinates; // Ring[]
  } else {
    const islands = feature.geometry.coordinates; // Ring[][]
  }
}
```

Required properties are non-optional; schema-optional ones (`kind`, `name_en`,
`code`, `wikidata`, `parent_id`, `ancestors`, `children`, ...) use `?`.

## Dart — `territory.dart`

No pubspec or third-party packages are needed: decode with `jsonDecode` from
`dart:convert` and pass the result to a `fromJson` factory.

```dart
import 'dart:convert';
import 'package:my_app/territory.dart'; // or a relative path in this repo

final entries = (jsonDecode(responseBody) as List<dynamic>)
    .map(IndexEntry.fromJson)
    .toList();

final collection = FeatureCollection.fromJson(jsonDecode(geojsonBody));
for (final feature in collection.features) {
  final geometry = feature.geometry;
  if (geometry is Polygon) {
    final rings = geometry.coordinates; // List<Ring>
  } else if (geometry is MultiPolygon) {
    final islands = geometry.coordinates; // List<List<Ring>>
  }
}
final json = jsonEncode(collection.toJson()); // snake_case keys preserved
```

`osm_id`/`parent_id`/`admin_level` decode to `int`, coordinates decode to
`double` (a JSON integer such as `113` is accepted and becomes `113.0`), and
`name` is a required `String` that may be empty. Optional properties are
nullable and omitted from `toJson()` when absent. Other GeoJSON geometry types
raise `FormatException`. See [`territory_test.dart`](territory_test.dart) for a
runnable example.

## Local verification

From the repository root:

```sh
# Schema vs. emitted JSON, and the TS name drift guard:
go test ./internal/emit/...

# Dart: static analysis and standalone tests (needs a Dart 3 SDK):
dart analyze types/territory.dart types/territory_test.dart
dart types/territory_test.dart

# TypeScript: strict compile plus runtime checks (no package.json needed):
npx --yes --package typescript@5.9.3 tsc --strict --target es2020 \
  --module commonjs --moduleResolution node --noEmit types/territory.test.ts
npx --yes --package typescript@5.9.3 tsc --strict --target es2020 \
  --module commonjs --moduleResolution node --outDir /tmp/ts-test types/territory.test.ts
node /tmp/ts-test/territory.test.js
```

The same Dart and TypeScript steps run in the `models` job of
[`.github/workflows/ci.yml`](../.github/workflows/ci.yml).

## Fixture provenance

The files in [`testdata/`](testdata/) are verbatim copies of index entries,
GeoJSON collections and the manifest from the published v2 data release
`data-2026-11-15`. They cover both geometry kinds, whole-number coordinates,
large numeric ids, empty names and present/absent optional properties.
