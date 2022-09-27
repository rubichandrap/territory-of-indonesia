class Geometry {
  String? type;
  dynamic coordinates;
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

class Features {
  String? type;
  Geometry? geometry;
  Properties? properties;
}

class Boundaries {
  String? type;
  List<Features>? features;
}
