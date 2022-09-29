package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"strconv"
	"strings"

	ct "territory-of-indonesia/constants"
	"territory-of-indonesia/generators"
	"territory-of-indonesia/interfaces"
)

func main() {
	opts := interfaces.ArgumentOptions{
		WithGeometry: true,
	}

	args := os.Args[1:]
	for _, arg := range args {
		if strings.Contains(arg, "--withGeometry=") {
			v := strings.Split(arg, "=")[1]
			if v != "true" && v != "false" {
				fmt.Println("invalid argument received by --withGeometry, the default value (true) will be used")
			} else {
				boolValue, err := strconv.ParseBool(v)
				if err != nil {
					log.Fatal(fmt.Errorf("error when parsing --withGeometry value too bool: %v", err))
				}
				opts.WithGeometry = boolValue
			}
		}
	}

	if _, err := os.Stat(ct.Path); os.IsNotExist(err) {
		err := os.Mkdir(ct.Path, os.ModeDir)
		if err != nil {
			log.Fatal(fmt.Errorf("error when making a dir: %v", err))
		}
	}

	c, err := os.ReadFile(ct.SourceFile)
	if err != nil {
		log.Fatal(fmt.Errorf("error when reading a file: %v", err))
	}

	var d interfaces.Boundaries

	err = json.Unmarshal(c, &d)
	if err != nil {
		log.Fatal(fmt.Errorf("error when unmarshaling: %v", err))
	}

	for _, feat := range d.Features {
		adminLevel, err := feat.Properties.AdminLevel.Int64()
		if err != nil {
			log.Fatal(fmt.Errorf("error when converting json number to int: %v", err))
		}

		parentId := strings.Split(feat.Properties.Parents, ",")[0]

		if adminLevel == ct.Province {
			generators.Generate(feat, ct.Path, "provinces.json", opts)
		} else if adminLevel == ct.District {
			generators.Generate(feat, ct.Path+"/districts", parentId+".json", opts)
		} else if adminLevel == ct.SubDistrict {
			generators.Generate(feat, ct.Path+"/sub_districts", parentId+".json", opts)
		}
	}
}
