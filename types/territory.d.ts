/**
 * Types for the Territory of Indonesia v2 dataset.
 *
 * schema/territory.schema.json is the normative contract. A Go test
 * (internal/emit/types_test.go) fails when a property in the schema is missing
 * here, so this file cannot silently fall behind.
 */

export type LevelName = "province" | "regency" | "district" | "village";

/** OpenStreetMap admin_level: 4 province, 5 kabupaten/kota, 6 kecamatan, 7 desa/kelurahan. */
export type AdminLevel = 4 | 5 | 6 | 7;

/** Directory names under index/, simplified/ and full/. */
export type ChildLevel = "regencies" | "districts" | "villages";

/** [longitude, latitude] */
export type Position = [longitude: number, latitude: number];

/** [west, south, east, north] */
export type BBox = [west: number, south: number, east: number, north: number];

/** Attributes of one region: the properties of every GeoJSON feature and the base of every index entry. */
export interface Properties {
  /** OpenStreetMap relation id. Unique across the dataset. */
  osm_id: number;
  level: LevelName;
  admin_level: AdminLevel;
  /** Local term: kabupaten, kota, kecamatan, distrik, desa, kelurahan, ... Absent when unknown. */
  kind?: string;
  /** OpenStreetMap name: the local name, often without the kabupaten/kota/kecamatan/desa prefix. */
  name: string;
  name_en?: string;
  /** Name including the type prefix, for example "Kota Bandung". */
  official_name?: string;
  /** Kemendagri code: NN, NN.NN, NN.NN.NN or NN.NN.NN.NNNN. Absent when OpenStreetMap has none. */
  code?: string;
  wikidata?: string;
  /**
   * osm_id of the nearest ancestor in the dataset. Normally the level above,
   * higher when OpenStreetMap has no region in between. Absent for provinces.
   */
  parent_id?: number;
  /** osm_id of every ancestor, province first. */
  ancestors?: number[];
}

/** One element of the arrays in index/*.json. */
export interface IndexEntry extends Properties {
  bbox: BBox;
  /** A point inside the region. */
  center: Position;
  /** Number of direct children per level; the file is index/<level>/<osm_id>.json. */
  children?: Partial<Record<ChildLevel, number>>;
}

export type Ring = Position[];

export interface Polygon {
  type: "Polygon";
  coordinates: Ring[];
}

export interface MultiPolygon {
  type: "MultiPolygon";
  coordinates: Ring[][];
}

/** A region in full/ or simplified/. */
export interface Feature {
  type: "Feature";
  /** Same as properties.osm_id. */
  id: number;
  bbox: BBox;
  properties: Properties;
  geometry: Polygon | MultiPolygon;
}

export interface FeatureCollection {
  type: "FeatureCollection";
  features: Feature[];
}

export interface Manifest {
  schema_version: 2;
  source: {
    name: string;
    provider: string;
    /** The data is at least this current: the oldest osm_timestamp of the extracts, UTC. */
    osm_timestamp?: string;
    /** The input files. A boundary on a border is completed from the neighbouring country's extract. */
    extracts?: {
      file: string;
      url?: string;
      sha256: string;
      osm_timestamp?: string;
    }[];
  };
  tool: string;
  counts: {
    provinces: number;
    regencies: number;
    districts: number;
    villages: number;
  };
  tiers: Record<
    string,
    {
      dir: string;
      description: string;
      decimals?: number;
      simplify_tolerance_degrees?: Record<string, number>;
      files: number;
      bytes: number;
    }
  >;
  license: "ODbL-1.0";
  attribution: string;
}
