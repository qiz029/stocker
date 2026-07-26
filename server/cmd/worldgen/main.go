package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

func main() {
	seed := flag.Uint64("seed", 42, "room seed")
	flag.Parse()
	w, err := engine.GenerateWorld(scenario.Synthetic(), *seed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fidelity gate failed:", err)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(w)
}
