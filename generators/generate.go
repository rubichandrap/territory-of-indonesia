package generators

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	ct "territory-of-indonesia/constants"
	"territory-of-indonesia/interfaces"
)

func Generate(data interfaces.Features, dirname string, filename string, opts ct.ArgumentOptions) {
	if _, err := os.Stat(dirname); os.IsNotExist(err) {
		err := os.Mkdir(dirname, os.ModeDir)
		if err != nil {
			log.Fatal(fmt.Errorf("error when making a dir: %v", err))
		}
	}

	var finalizedData interface{} = data

	if !opts.WithGeometry {
		finalizedData = data
	} else if opts.WithGMapsPolygonFormat {
		geo := data.Geometry.(interfaces.Geometry)
		if len(geo.Coordinates) > 0 {
			coords := geo.Coordinates[0].([][]json.Number)
			for idx, coord := range coords {
				geo.Coordinates[idx] = interfaces.LatLng{
					Lat: coord[0],
					Lng: coord[1],
				}
			}
		}

		finalizedData = geo
	}

	fullPath := dirname + "/" + filename

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		bounds := interfaces.Boundaries{
			Type: "FeatureCollection",
		}

		bounds.Features = append(bounds.Features, finalizedData.(interfaces.Features))

		file, err := json.Marshal(&bounds)
		if err != nil {
			log.Fatal(fmt.Errorf("error when marshaling: %v", err))
		}

		err = os.WriteFile(fullPath, file, 0644)
		if err != nil {
			log.Fatal(fmt.Errorf("error when writing a file: %v", err))
		}
	} else {
		c, err := os.ReadFile(fullPath)
		if err != nil {
			log.Fatal(fmt.Errorf("error when reading a file: %v", err))
		}

		var bounds interfaces.Boundaries

		err = json.Unmarshal(c, &bounds)
		if err != nil {
			log.Fatal(fmt.Errorf("error when unmarshaling: %v", err))
		}

		bounds.Features = append(bounds.Features, finalizedData.(interfaces.Features))

		file, err := json.Marshal(finalizedData.(interfaces.Features))
		if err != nil {
			log.Fatal(fmt.Errorf("error when marshaling: %v", err))
		}

		err = os.WriteFile(fullPath, file, 0644)
		if err != nil {
			log.Fatal(fmt.Errorf("error when writing a file: %v", err))
		}
	}
}
