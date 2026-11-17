// Runnable tests and usage examples for types/territory.dart.
//
// Run from the repository root with any Dart 3 SDK (no pubspec/packages):
//   dart types/territory_test.dart
//
// The fixtures in testdata/ are verbatim files from the published v2 release,
// so this both demonstrates the API and checks decoding of real emitted JSON.

import 'dart:convert';
import 'dart:io';

import 'territory.dart';

int _checks = 0;

void check(bool condition, String message) {
  _checks++;
  if (!condition) throw StateError('FAIL: $message');
}

final Directory _testdata = Directory('types/testdata');

Map<String, dynamic> readJson(String name) {
  final file = File(_testdata.uri.resolve(name).toFilePath());
  return jsonDecode(file.readAsStringSync()) as Map<String, dynamic>;
}

List<dynamic> readJsonList(String name) {
  final file = File(_testdata.uri.resolve(name).toFilePath());
  return jsonDecode(file.readAsStringSync()) as List<dynamic>;
}

/// Recursive semantic JSON equality: object key order is irrelevant and a
/// whole JSON number written as an int equals the same value written as a
/// double (the emitter may write `113` where toJson writes `113.0`).
bool jsonEqual(Object? a, Object? b) {
  if (a is num && b is num) {
    if (a is int && b is int) return a == b;
    return a.toDouble() == b.toDouble();
  }
  if (a is Map && b is Map) {
    if (a.length != b.length) return false;
    for (final key in a.keys) {
      if (!b.containsKey(key) || !jsonEqual(a[key], b[key])) return false;
    }
    return true;
  }
  if (a is List && b is List) {
    if (a.length != b.length) return false;
    for (var i = 0; i < a.length; i++) {
      if (!jsonEqual(a[i], b[i])) return false;
    }
    return true;
  }
  return a == b;
}

/// Decoding and re-encoding must reproduce the original JSON semantically, and
/// a second decode/encode cycle must be stable. Comparing only the first and
/// second encodings would let a decoder that silently drops a field pass.
void roundTrip<T>(
  Object? original,
  T Function(Object? json) decode,
  Object Function(T value) encode,
  String label,
) {
  final once = decode(original);
  final onceJson = encode(once);
  check(
    jsonEqual(original, onceJson),
    '$label: decode/toJson changed or lost data from the original JSON',
  );
  final twice = decode(jsonDecode(jsonEncode(onceJson)));
  final twiceJson = encode(twice);
  check(
    jsonEqual(onceJson, twiceJson),
    '$label: toJson is not stable across a round trip',
  );
}

// ---------------------------------------------------------------------------
// Features: both geometry kinds, integer coordinates, optionals, numeric ids.
// ---------------------------------------------------------------------------

void testPolygon() {
  final raw = readJson('polygon.geojson');
  final collection = FeatureCollection.fromJson(raw);
  check(collection.features.isNotEmpty, 'polygon fixture has no features');
  final feature = collection.features.first;
  check(feature.id == feature.properties.osmId, 'feature id must equal osm_id');
  check(feature.id > 1_000_000, 'id is a large integer');
  check(feature.properties.level == LevelName.village, 'level decoded to the enum');
  check(feature.properties.kind == null, 'absent optional kind stays null');
  check(feature.properties.wikidata == null, 'absent optional wikidata stays null');

  final geometry = feature.geometry;
  check(geometry is Polygon, 'expected a typed Polygon, got ${geometry.runtimeType}');
  if (geometry is Polygon) {
    check(geometry.coordinates.isNotEmpty, 'polygon has no rings');
    final outer = geometry.coordinates.first;
    check(outer.length >= 4, 'a ring needs at least four positions');
    check(outer.first.latitude.isFinite, 'ring members are typed numeric Positions');

    // The real fixture has a whole-number coordinate written as a JSON int.
    final rawRing = (raw['features'][0]['geometry']['coordinates'] as List<dynamic>)[0]
        as List<dynamic>;
    final rawWhole = rawRing.firstWhere((p) => (p as List<dynamic>)[0] is int)
        as List<dynamic>;
    final int rawValue = rawWhole[0] as int;
    check(
      outer.any((p) => p.longitude == rawValue.toDouble() && p.longitude.isFinite),
      'integer JSON coordinate $rawValue decoded to the same double',
    );
  }

  roundTrip(readJson('polygon.geojson'), FeatureCollection.fromJson,
      (c) => c.toJson(), 'polygon.geojson');

  // Example: typed access without a single `dynamic` cast.
  for (final f in collection.features) {
    final g = f.geometry;
    if (g is Polygon) {
      check(g.coordinates.first.isNotEmpty, 'every polygon has a first ring');
    } else if (g is MultiPolygon) {
      check(g.coordinates.isNotEmpty, 'every multipolygon has at least one island');
    }
  }
}

void testMultiPolygon() {
  final collection = FeatureCollection.fromJson(readJson('multipolygon.geojson'));
  final geometry = collection.features.first.geometry;
  check(geometry is MultiPolygon, 'expected a typed MultiPolygon');
  if (geometry is MultiPolygon) {
    check(geometry.coordinates.length > 1, 'the real Raijua fixture has several islands');
    check(geometry.coordinates.first.first.isNotEmpty, 'islands contain typed rings');
  }
  roundTrip(readJson('multipolygon.geojson'), FeatureCollection.fromJson,
      (c) => c.toJson(), 'multipolygon.geojson');
}

void testMixedGeometries() {
  final collection = FeatureCollection.fromJson(readJson('mixed.geojson'));
  final kinds = collection.features.map((f) => f.geometry.runtimeType).toSet();
  check(kinds.contains(Polygon), 'mixed fixture should contain a Polygon');
  check(kinds.contains(MultiPolygon), 'mixed fixture should contain a MultiPolygon');
  roundTrip(readJson('mixed.geojson'), FeatureCollection.fromJson,
      (c) => c.toJson(), 'mixed.geojson');
}

// ---------------------------------------------------------------------------
// Index entries: numeric ids, empty names, optional properties.
// ---------------------------------------------------------------------------

void testIndexEntries() {
  final provinces = readJsonList('index-provinces.json')
      .map((e) => IndexEntry.fromJson(e))
      .toList();
  check(provinces.isNotEmpty, 'provinces fixture is empty');
  final first = provinces.first;
  check(first.osmId == 2390836, 'first real osm_id is numeric');
  check(first.level == LevelName.province && first.adminLevel == 4, 'required level fields');
  check(first.name.isNotEmpty, 'this province has a name');
  check(first.parentId == null && first.ancestors == null, 'provinces have no ancestors');
  check(first.children != null && first.children!.containsKey('regencies'), 'child counts decoded');
  check(first.bbox.length == 4 && first.center.length == 2, 'bbox/center arity');
  check(provinces.every((e) => e.name.isNotEmpty), 'a name is always a string, empty allowed');

  final districts = readJsonList('index-empty-name.json')
      .map((e) => IndexEntry.fromJson(e))
      .toList();
  final blank = districts.where((e) => e.name.isEmpty).toList();
  check(blank.isNotEmpty, 'real fixture has an empty-name district');
  check(blank.first.osmId == 19188597, 'empty-name region has a large numeric id');
  check(blank.first.wikidata == 'Q12518690', 'optional wikidata decoded');
  check(blank.first.children?['villages'] == 14, 'optional children decoded');
  check(blank.first.code == null, 'absent optional code stays null');

  roundTrip<List<IndexEntry>>(
    readJsonList('index-provinces.json'),
    (json) => (json as List<dynamic>).map(IndexEntry.fromJson).toList(),
    (entries) => entries.map((e) => e.toJson()).toList(),
    'index-provinces.json',
  );
  roundTrip<List<IndexEntry>>(
    readJsonList('index-empty-name.json'),
    (json) => (json as List<dynamic>).map(IndexEntry.fromJson).toList(),
    (entries) => entries.map((e) => e.toJson()).toList(),
    'index-empty-name.json',
  );
}

// ---------------------------------------------------------------------------
// Manifest: required constants plus optional provenance fields.
// ---------------------------------------------------------------------------

void testManifest() {
  final manifest = Manifest.fromJson(readJson('manifest.json'));
  check(manifest.schemaVersion == 2, 'schema_version is the literal 2');
  check(manifest.license == 'ODbL-1.0', 'license is the schema constant');
  check(manifest.source.osmTimestamp != null, 'published manifest has osm_timestamp');
  check(manifest.source.extracts != null && manifest.source.extracts!.isNotEmpty, 'extracts decoded');
  check(manifest.counts['provinces'] == 38, 'counts are integers');
  check(manifest.tiers['simplified']?.decimals == 5, 'optional tier decimals decoded');
  check(manifest.tiers['simplified']?.simplifyToleranceDegrees != null, 'optional tolerance decoded');
  roundTrip(readJson('manifest.json'), Manifest.fromJson,
      (m) => m.toJson(), 'manifest.json');
}

// ---------------------------------------------------------------------------
// Malformed input is rejected instead of being silently treated as dynamic.
// ---------------------------------------------------------------------------

void testRejectsBadInput() {
  bool throwsFormat(void Function() body) {
    try {
      body();
      return false;
    } on FormatException {
      return true;
    }
  }

  check(throwsFormat(() => Geometry.fromJson({'type': 'Point'})), 'unknown geometry type is rejected');
  check(throwsFormat(() => Position.fromJson([1])), 'short position is rejected');
  check(throwsFormat(() => LevelName.fromJson('provinsi')), 'unknown level is rejected');
  check(throwsFormat(() => FeatureCollection.fromJson({'type': 'Feature'})), 'wrong document type is rejected');
}

void testSnakeCaseKeys() {
  final entry = IndexEntry.fromJson(readJsonList('index-empty-name.json').first);
  final props = entry.toJson();
  for (final key in ['osm_id', 'level', 'admin_level', 'name', 'parent_id']) {
    check(props.containsKey(key), 'properties must use the snake_case key $key');
  }
  for (final camel in ['osmId', 'adminLevel', 'parentId', 'nameEn', 'officialName']) {
    check(!props.containsKey(camel), 'camelCase key $camel leaked into JSON');
  }

  final feature = FeatureCollection.fromJson(readJson('polygon.geojson')).features.first;
  for (final key in ['osm_id', 'admin_level', 'parent_id']) {
    check(feature.properties.toJson().containsKey(key), 'feature property $key');
  }
  check(feature.toJson().containsKey('geometry') && feature.geometry.toJson().containsKey('coordinates'),
      'geometry uses GeoJSON keys');

  final manifest = Manifest.fromJson(readJson('manifest.json')).toJson();
  for (final key in ['schema_version', 'osm_timestamp', 'simplify_tolerance_degrees']) {
    check(_deepContainsKey(manifest, key), 'manifest must use the snake_case key $key');
  }
  for (final camel in ['schemaVersion', 'osmTimestamp', 'simplifyToleranceDegrees']) {
    check(!_deepContainsKey(manifest, camel), 'camelCase key $camel leaked into manifest JSON');
  }
}

bool _deepContainsKey(Object? value, String key) {
  if (value is Map<String, dynamic>) {
    return value.containsKey(key) || value.values.any((v) => _deepContainsKey(v, key));
  }
  if (value is List) return value.any((v) => _deepContainsKey(v, key));
  return false;
}

/// Negative control: a missing optional key and a reordered object with
/// int/double-equivalent numbers must be treated differently.
void testRoundTripDetectsDataLoss() {
  final Map<String, dynamic> original = <String, dynamic>{
    'osm_id': 1,
    'level': 'village',
    'admin_level': 7,
    'name': 'x',
    'wikidata': 'Q1',
  };
  final dropped = Map<String, dynamic>.from(original)..remove('wikidata');
  check(!jsonEqual(original, dropped), 'semantic equality must catch a missing optional key');
  check(
    jsonEqual(original, <String, dynamic>{
      'name': 'x',
      'wikidata': 'Q1',
      'level': 'village',
      'admin_level': 7.0,
      'osm_id': 1.0,
    }),
    'semantic equality must ignore key order and int/double differences',
  );

  var threw = false;
  try {
    roundTrip<Map<String, dynamic>>(
      original,
      (json) => Map<String, dynamic>.from(json as Map),
      (value) => value..remove('wikidata'),
      'negative control',
    );
  } on StateError {
    threw = true;
  }
  check(threw, 'roundTrip must fail when the encoder drops a field');
}

void main() {
  testRoundTripDetectsDataLoss();
  testSnakeCaseKeys();
  testPolygon();
  testMultiPolygon();
  testMixedGeometries();
  testIndexEntries();
  testManifest();
  testRejectsBadInput();
  stdout.writeln('territory_test.dart: $_checks checks passed against real v2 fixtures');
}
