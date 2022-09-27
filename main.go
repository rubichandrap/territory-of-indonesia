package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"territory-of-indonesia/generators"
	"territory-of-indonesia/interfaces"
)

const (
	Province int64 = iota + 4
	District
	SubDistrict
)

const (
	Path       string = "./dist"
	SourceFile string = "./osm-boundaries.json"
)

func main() {
	c, err := os.ReadFile(SourceFile)
	if err != nil {
		log.Fatal(fmt.Errorf("error when reading a file: %v", err))
	}

	var d interfaces.Boundaries

	err = json.Unmarshal(c, &d)
	if err != nil {
		log.Fatal(fmt.Errorf("error when unmarshaling: %v", err))
	}

	if _, err := os.Stat(Path); os.IsNotExist(err) {
		err := os.Mkdir(Path, os.ModeDir)
		if err != nil {
			log.Fatal(fmt.Errorf("error when making a dir: %v", err))
		}
	}

	for _, feat := range d.Features {
		adminLevel, err := feat.Properties.AdminLevel.Int64()
		if err != nil {
			log.Fatal(fmt.Errorf("error when converting json number to int: %v", err))
		}

		parentId := strings.Split(feat.Properties.Parents, ",")[0]

		if adminLevel == Province {
			generators.Generate(feat, Path, "provinces.json")
		} else if adminLevel == District {
			generators.Generate(feat, Path+"/districts", parentId+".json")
		} else if adminLevel == SubDistrict {
			generators.Generate(feat, Path+"/sub_districts", parentId+".json")
		}
	}
}
