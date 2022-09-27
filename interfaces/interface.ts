export interface Geometry {
  type: string;
  coordinates: number[][][];
}

export interface Properties {
  osmID: number;
  boundary: string;
  adminLevel: number;
  parents: string;
  name: string;
  localName: string;
  nameEN: string;
}

export interface BoundaryFeatures {
  type: string;
  geometry: Geometry;
  properties: Properties;
}

export interface Boundaries {
  type: string;
  features: BoundaryFeatures[];
}