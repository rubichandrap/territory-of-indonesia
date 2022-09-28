package constants

type ArgumentOptions struct {
	WithGeometry           bool
	WithGMapsPolygonFormat bool
}

const (
	Province int64 = iota + 4
	District
	SubDistrict

	Path       string = "./dist"
	SourceFile string = "./osm-boundaries.json"
)
