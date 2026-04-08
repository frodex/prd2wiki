package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	configPath := flag.String("config", "config/prd2wiki.yaml", "path to config file")
	flag.Parse()

	fmt.Printf("prd2wiki starting with config: %s\n", *configPath)
	os.Exit(0)
}
