/// Typed decoding for the Territory of Indonesia v2 dataset.
///
/// The JSON keys here are the snake_case keys the emitter actually ships
/// (normative contract: schema/territory.schema.json). Required properties are
/// non-nullable; optional properties are nullable and are omitted from
/// [toJson] when absent, so a decode/encode round trip keeps the original
/// shape. Geometry is a typed [Polygon]/[MultiPolygon] union, never `dynamic`.
///
/// Only dart:core is required; decode with `jsonDecode` from dart:convert and
/// hand the result to [FeatureCollection.fromJson], [IndexEntry.fromJson] or
/// [Manifest.fromJson].

// ---------------------------------------------------------------------------
// JSON value helpers. They turn unchecked dynamic data into typed values and
// name the offending field when the input does not match the schema.
// ---------------------------------------------------------------------------

int _asInt(Object? value, String field) {
  if (value is int) return value;
  throw FormatException('$field must be an integer, got ${value.runtimeType}: $value');
}

double _asDouble(Object? value, String field) {
  // JSON decodes whole numbers as int and decimals as double. GeoJSON accepts
  // both, so an integer coordinate is a perfectly valid number here.
  if (value is num) return value.toDouble();
  throw FormatException('$field must be a number, got ${value.runtimeType}: $value');
}

String _asString(Object? value, String field) {
  if (value is String) return value; // the empty string is a valid name
  throw FormatException('$field must be a string, got ${value.runtimeType}: $value');
}

Map<String, dynamic> _asObject(Object? value, String field) {
  if (value is Map<String, dynamic>) return value;
  throw FormatException('$field must be a JSON object, got ${value.runtimeType}');
}

List<dynamic> _asList(Object? value, String field) {
  if (value is List) return value;
  throw FormatException('$field must be a JSON array, got ${value.runtimeType}');
}

List<double> _numbers(Object? value, String field, int length) {
  final list = _asList(value, field);
  if (list.length != length) {
    throw FormatException('$field must have $length numbers, got ${list.length}');
  }
  return list.map((v) => _asDouble(v, field)).toList(growable: false);
}

String? _optionalString(Map<String, dynamic> json, String key) =>
    json.containsKey(key) ? _asString(json[key], key) : null;

int? _optionalInt(Map<String, dynamic> json, String key) =>
    json.containsKey(key) ? _asInt(json[key], key) : null;

List<int>? _optionalInts(Map<String, dynamic> json, String key) {
  if (!json.containsKey(key)) return null;
  return _asList(json[key], key)
      .map((v) => _asInt(v, key))
      .toList(growable: false);
}

// ---------------------------------------------------------------------------
// Administrative level and geometry.
// ---------------------------------------------------------------------------

/// The `level` property: one of the four published levels.
enum LevelName {
  province('province'),
  regency('regency'),
  district('district'),
  village('village');

  const LevelName(this.json);

  /// The exact string used in the emitted JSON.
  final String json;

  static LevelName fromJson(Object? value) {
    for (final level in values) {
      if (level.json == value) return level;
    }
    throw FormatException('unknown level: $value');
  }
}

/// [longitude, latitude], both numbers.
class Position {
  final double longitude;
  final double latitude;

  const Position(this.longitude, this.latitude);

  factory Position.fromJson(Object? value) {
    final pair = _asList(value, 'position');
    if (pair.length != 2) {
      throw FormatException('position must have 2 numbers, got ${pair.length}');
    }
    return Position(_asDouble(pair[0], 'longitude'), _asDouble(pair[1], 'latitude'));
  }

  List<dynamic> toJson() => <dynamic>[longitude, latitude];

  @override
  String toString() => 'Position($longitude, $latitude)';
}

/// A closed linear ring: at least four positions, first equal to last.
typedef Ring = List<Position>;

/// A polygon is its exterior ring followed by any interior rings (holes).
typedef PolygonRings = List<Ring>;

/// [west, south, east, north].
typedef BBox = List<double>;

Ring _ring(Object? value) => _asList(value, 'ring')
    .map((position) => Position.fromJson(position))
    .toList(growable: false);

PolygonRings _rings(Object? value) =>
    _asList(value, 'coordinates').map(_ring).toList(growable: false);

/// GeoJSON geometry, restricted to the two kinds this dataset emits.
abstract class Geometry {
  const Geometry();

  /// Dispatches on the GeoJSON `type` discriminator.
  factory Geometry.fromJson(Object? value) {
    final json = _asObject(value, 'geometry');
    switch (json['type']) {
      case 'Polygon':
        return Polygon.fromJson(json);
      case 'MultiPolygon':
        return MultiPolygon.fromJson(json);
      default:
        throw FormatException('unsupported geometry type: ${json['type']}');
    }
  }

  Map<String, dynamic> toJson();
}

class Polygon extends Geometry {
  /// Exterior ring first, then any holes.
  final PolygonRings coordinates;

  const Polygon(this.coordinates);

  factory Polygon.fromJson(Map<String, dynamic> json) {
    if (json['type'] != 'Polygon') {
      throw FormatException('not a Polygon: ${json['type']}');
    }
    return Polygon(_rings(json['coordinates']));
  }

  @override
  Map<String, dynamic> toJson() => <String, dynamic>{
        'type': 'Polygon',
        'coordinates': coordinates
            .map((ring) => ring.map((position) => position.toJson()).toList())
            .toList(),
      };

  @override
  String toString() => 'Polygon(${coordinates.length} rings)';
}

class MultiPolygon extends Geometry {
  /// One list of rings per island.
  final List<PolygonRings> coordinates;

  const MultiPolygon(this.coordinates);

  factory MultiPolygon.fromJson(Map<String, dynamic> json) {
    if (json['type'] != 'MultiPolygon') {
      throw FormatException('not a MultiPolygon: ${json['type']}');
    }
    return MultiPolygon(_asList(json['coordinates'], 'coordinates')
        .map(_rings)
        .toList(growable: false));
  }

  @override
  Map<String, dynamic> toJson() => <String, dynamic>{
        'type': 'MultiPolygon',
        'coordinates': coordinates
            .map((polygon) => polygon
                .map((ring) => ring.map((position) => position.toJson()).toList())
                .toList())
            .toList(),
      };

  @override
  String toString() => 'MultiPolygon(${coordinates.length} polygons)';
}

// ---------------------------------------------------------------------------
// Properties and the three top-level document types.
// ---------------------------------------------------------------------------

/// Attributes shared by every index entry and GeoJSON feature.
class Properties {
  final int osmId; // json: osm_id
  final LevelName level;
  final int adminLevel; // json: admin_level
  final String name; // required, but may be the empty string
  final String? kind;
  final String? nameEn; // json: name_en
  final String? officialName; // json: official_name
  final String? code;
  final String? wikidata;
  final int? parentId; // json: parent_id
  final List<int>? ancestors;

  const Properties({
    required this.osmId,
    required this.level,
    required this.adminLevel,
    required this.name,
    this.kind,
    this.nameEn,
    this.officialName,
    this.code,
    this.wikidata,
    this.parentId,
    this.ancestors,
  });

  factory Properties.fromJson(Object? value) => parsed(_asObject(value, 'properties'));

  /// Shared by [Properties.fromJson] and [IndexEntry.fromJson].
  static Properties parsed(Map<String, dynamic> json) => Properties(
        osmId: _asInt(json['osm_id'], 'osm_id'),
        level: LevelName.fromJson(json['level']),
        adminLevel: _asInt(json['admin_level'], 'admin_level'),
        name: _asString(json['name'], 'name'),
        kind: _optionalString(json, 'kind'),
        nameEn: _optionalString(json, 'name_en'),
        officialName: _optionalString(json, 'official_name'),
        code: _optionalString(json, 'code'),
        wikidata: _optionalString(json, 'wikidata'),
        parentId: _optionalInt(json, 'parent_id'),
        ancestors: _optionalInts(json, 'ancestors'),
      );

  /// Rebuild the exact snake_case shape that was decoded.
  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{
      'osm_id': osmId,
      'level': level.json,
      'admin_level': adminLevel,
      'name': name,
    };
    if (kind != null) json['kind'] = kind;
    if (nameEn != null) json['name_en'] = nameEn;
    if (officialName != null) json['official_name'] = officialName;
    if (code != null) json['code'] = code;
    if (wikidata != null) json['wikidata'] = wikidata;
    if (parentId != null) json['parent_id'] = parentId;
    if (ancestors != null) json['ancestors'] = List<int>.from(ancestors!);
    return json;
  }
}

const Set<String> _childLevels = <String>{'regencies', 'districts', 'villages'};

/// One element of an index/*.json array.
class IndexEntry extends Properties {
  final BBox bbox;
  final BBox center;
  final Map<String, int>? children;

  const IndexEntry({
    required super.osmId,
    required super.level,
    required super.adminLevel,
    required super.name,
    super.kind,
    super.nameEn,
    super.officialName,
    super.code,
    super.wikidata,
    super.parentId,
    super.ancestors,
    required this.bbox,
    required this.center,
    this.children,
  });

  factory IndexEntry.fromJson(Object? value) {
    final json = _asObject(value, 'index entry');
    final p = Properties.parsed(json);
    Map<String, int>? children;
    if (json.containsKey('children')) {
      final raw = _asObject(json['children'], 'children');
      children = <String, int>{
        for (final entry in raw.entries)
          entry.key: _childLevels.contains(entry.key)
              ? _asInt(entry.value, 'children.${entry.key}')
              : (throw FormatException('unknown child level: ${entry.key}')),
      };
    }
    return IndexEntry(
      osmId: p.osmId,
      level: p.level,
      adminLevel: p.adminLevel,
      name: p.name,
      kind: p.kind,
      nameEn: p.nameEn,
      officialName: p.officialName,
      code: p.code,
      wikidata: p.wikidata,
      parentId: p.parentId,
      ancestors: p.ancestors,
      bbox: _numbers(json['bbox'], 'bbox', 4),
      center: _numbers(json['center'], 'center', 2),
      children: children,
    );
  }

  @override
  Map<String, dynamic> toJson() {
    final json = super.toJson();
    json['bbox'] = List<double>.from(bbox);
    json['center'] = List<double>.from(center);
    if (children != null) json['children'] = Map<String, int>.from(children!);
    return json;
  }
}

/// One feature of a full/ or simplified/ GeoJSON file.
class Feature {
  final int id; // same as properties.osm_id
  final BBox bbox;
  final Properties properties;
  final Geometry geometry;

  const Feature({
    required this.id,
    required this.bbox,
    required this.properties,
    required this.geometry,
  });

  factory Feature.fromJson(Object? value) {
    final json = _asObject(value, 'feature');
    if (json['type'] != 'Feature') {
      throw FormatException('not a Feature: ${json['type']}');
    }
    return Feature(
      id: _asInt(json['id'], 'id'),
      bbox: _numbers(json['bbox'], 'bbox', 4),
      properties: Properties.fromJson(json['properties']),
      geometry: Geometry.fromJson(json['geometry']),
    );
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
        'type': 'Feature',
        'id': id,
        'bbox': List<double>.from(bbox),
        'properties': properties.toJson(),
        'geometry': geometry.toJson(),
      };
}

class FeatureCollection {
  final List<Feature> features;

  const FeatureCollection(this.features);

  factory FeatureCollection.fromJson(Object? value) {
    final json = _asObject(value, 'FeatureCollection');
    if (json['type'] != 'FeatureCollection') {
      throw FormatException('not a FeatureCollection: ${json['type']}');
    }
    return FeatureCollection(_asList(json['features'], 'features')
        .map(Feature.fromJson)
        .toList(growable: false));
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
        'type': 'FeatureCollection',
        'features': features.map((feature) => feature.toJson()).toList(),
      };
}

/// One input file recorded in the manifest's provenance block.
class Extract {
  final String file;
  final String? url;
  final String sha256;
  final String? osmTimestamp; // json: osm_timestamp

  const Extract({
    required this.file,
    this.url,
    required this.sha256,
    this.osmTimestamp,
  });

  factory Extract.fromJson(Object? value) {
    final json = _asObject(value, 'extract');
    return Extract(
      file: _asString(json['file'], 'file'),
      url: _optionalString(json, 'url'),
      sha256: _asString(json['sha256'], 'sha256'),
      osmTimestamp: _optionalString(json, 'osm_timestamp'),
    );
  }

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{'file': file};
    if (url != null) json['url'] = url;
    json['sha256'] = sha256;
    if (osmTimestamp != null) json['osm_timestamp'] = osmTimestamp;
    return json;
  }
}

class Source {
  final String name;
  final String provider;
  final String? osmTimestamp; // json: osm_timestamp
  final List<Extract>? extracts;

  const Source({
    required this.name,
    required this.provider,
    this.osmTimestamp,
    this.extracts,
  });

  factory Source.fromJson(Object? value) {
    final json = _asObject(value, 'source');
    return Source(
      name: _asString(json['name'], 'name'),
      provider: _asString(json['provider'], 'provider'),
      osmTimestamp: _optionalString(json, 'osm_timestamp'),
      extracts: json.containsKey('extracts')
          ? _asList(json['extracts'], 'extracts')
              .map(Extract.fromJson)
              .toList(growable: false)
          : null,
    );
  }

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{'name': name, 'provider': provider};
    if (osmTimestamp != null) json['osm_timestamp'] = osmTimestamp;
    if (extracts != null) {
      json['extracts'] = extracts!.map((extract) => extract.toJson()).toList();
    }
    return json;
  }
}

/// Metadata for one output tier (index, simplified or full).
class TierInfo {
  final String dir;
  final String description;
  final int? decimals;
  final Map<String, double>? simplifyToleranceDegrees; // json: simplify_tolerance_degrees
  final int files;
  final int bytes;

  const TierInfo({
    required this.dir,
    required this.description,
    this.decimals,
    this.simplifyToleranceDegrees,
    required this.files,
    required this.bytes,
  });

  factory TierInfo.fromJson(Object? value) {
    final json = _asObject(value, 'tier');
    Map<String, double>? tolerance;
    if (json.containsKey('simplify_tolerance_degrees')) {
      final raw = _asObject(json['simplify_tolerance_degrees'], 'simplify_tolerance_degrees');
      tolerance = <String, double>{
        for (final entry in raw.entries) entry.key: _asDouble(entry.value, 'simplify_tolerance_degrees.${entry.key}'),
      };
    }
    return TierInfo(
      dir: _asString(json['dir'], 'dir'),
      description: _asString(json['description'], 'description'),
      decimals: _optionalInt(json, 'decimals'),
      simplifyToleranceDegrees: tolerance,
      files: _asInt(json['files'], 'files'),
      bytes: _asInt(json['bytes'], 'bytes'),
    );
  }

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{
      'dir': dir,
      'description': description,
    };
    if (decimals != null) json['decimals'] = decimals;
    if (simplifyToleranceDegrees != null) {
      json['simplify_tolerance_degrees'] = Map<String, double>.from(simplifyToleranceDegrees!);
    }
    json['files'] = files;
    json['bytes'] = bytes;
    return json;
  }
}

class Manifest {
  final int schemaVersion; // json: schema_version
  final Source source;
  final String tool;
  final Map<String, int> counts;
  final Map<String, TierInfo> tiers;
  final String license;
  final String attribution;

  const Manifest({
    required this.schemaVersion,
    required this.source,
    required this.tool,
    required this.counts,
    required this.tiers,
    required this.license,
    required this.attribution,
  });

  factory Manifest.fromJson(Object? value) {
    final json = _asObject(value, 'manifest');
    final version = _asInt(json['schema_version'], 'schema_version');
    if (version != 2) throw FormatException('unsupported schema_version: $version');
    final license = _asString(json['license'], 'license');
    if (license != 'ODbL-1.0') throw FormatException('unexpected license: $license');
    return Manifest(
      schemaVersion: version,
      source: Source.fromJson(json['source']),
      tool: _asString(json['tool'], 'tool'),
      counts: <String, int>{
        for (final entry in _asObject(json['counts'], 'counts').entries)
          entry.key: _asInt(entry.value, 'counts.${entry.key}'),
      },
      tiers: <String, TierInfo>{
        for (final entry in _asObject(json['tiers'], 'tiers').entries)
          entry.key: TierInfo.fromJson(entry.value),
      },
      license: license,
      attribution: _asString(json['attribution'], 'attribution'),
    );
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
        'schema_version': schemaVersion,
        'source': source.toJson(),
        'tool': tool,
        'counts': Map<String, int>.from(counts),
        'tiers': <String, dynamic>{
          for (final entry in tiers.entries) entry.key: entry.value.toJson(),
        },
        'license': license,
        'attribution': attribution,
      };
}
