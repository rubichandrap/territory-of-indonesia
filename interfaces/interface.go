package interfaces

import "encoding/json"

type LatLng struct {
	Lat json.Number `json:"lat"`
	Lng json.Number `json:"lng"`
}

type Geometry struct {
	Type        string        `json:"type"`
	Coordinates []interface{} `json:"coordinates"`
}

type Properties struct {
	OSMID      json.Number `json:"osm_id"`
	Boundary   string      `json:"boundary"`
	AdminLevel json.Number `json:"admin_level"`
	Parents    string      `json:"parents"`
	Name       string      `json:"name"`
	LocalName  string      `json:"local_name"`
	NameEN     string      `json:"name_en"`
}

type Features struct {
	Type       string      `json:"type"`
	Geometry   interface{} `json:"geometry"`
	Properties Properties  `json:"properties"`
}

type Boundaries struct {
	Type     string     `json:"type"`
	Features []Features `json:"features"`
}
