package generators

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"territory-of-indonesia/interfaces"
)

func Generate(data interfaces.Features, dirname string, filename string, opts interfaces.ArgumentOptions) {
	if _, err := os.Stat(dirname); os.IsNotExist(err) {
		err := os.Mkdir(dirname, os.ModeDir)
		if err != nil {
			log.Fatal(fmt.Errorf("error when making a dir: %v", err))
		}
	}

	if !opts.WithGeometry {
		data.Geometry = nil
	}

	fp := dirname + "/" + filename

	if _, err := os.Stat(fp); os.IsNotExist(err) {
		bounds := interfaces.Boundaries{
			Type: "FeatureCollection",
		}

		bounds.Features = append(bounds.Features, data)

		file, err := json.Marshal(&bounds)
		if err != nil {
			log.Fatal(fmt.Errorf("error when marshaling: %v", err))
		}

		err = os.WriteFile(fp, file, 0644)
		if err != nil {
			log.Fatal(fmt.Errorf("error when writing a file: %v", err))
		}
	} else {
		c, err := os.ReadFile(fp)
		if err != nil {
			log.Fatal(fmt.Errorf("error when reading a file: %v", err))
		}

		var bounds interfaces.Boundaries

		err = json.Unmarshal(c, &bounds)
		if err != nil {
			log.Fatal(fmt.Errorf("error when unmarshaling: %v", err))
		}

		bounds.Features = append(bounds.Features, data)

		file, err := json.Marshal(&bounds)
		if err != nil {
			log.Fatal(fmt.Errorf("error when marshaling: %v", err))
		}

		err = os.WriteFile(fp, file, 0644)
		if err != nil {
			log.Fatal(fmt.Errorf("error when writing a file: %v", err))
		}
	}
}
