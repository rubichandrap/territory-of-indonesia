class Geometry {
  String? type;
  List<List<List<int>>>? coordinates;
}

class Properties {
  int? osmID;
  String? boundary;
  int? adminLevel;
  String? parents;
  String? name;
  String? localName;
  String? nameEN;
}

class BoundaryFeatures {
  String? type;
  Geometry? geometry;
  Properties? properties;
}

class Boundaries {
  String? type;
  List<BoundaryFeatures>? features;
}
