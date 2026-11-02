package main

import (
	"encoding/json"
	"github.com/polygon-zk-poker/server/internal/fairness"
	"log"
	"os"
)

func main() {
	a, _, e := fairness.SimulateNine()
	if e != nil {
		log.Fatal(e)
	}
	if _, e := fairness.Verify(a); e != nil {
		log.Fatal(e)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if e := enc.Encode(a); e != nil {
		log.Fatal(e)
	}
}
