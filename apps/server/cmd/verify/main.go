package main

import (
	"encoding/json"
	"fmt"
	"github.com/polygon-zk-poker/server/internal/fairness"
	"io"
	"os"
)

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: verify AUDIT.json")
	}
	f, e := os.Open(os.Args[1])
	if e != nil {
		return e
	}
	defer f.Close()
	// Never silently accept trailing JSON or unknown fields.
	d := json.NewDecoder(io.LimitReader(f, 16<<20))
	d.DisallowUnknownFields()
	var a fairness.Audit
	if e := d.Decode(&a); e != nil {
		return e
	}
	var tail any
	if e := d.Decode(&tail); e != io.EOF {
		return fmt.Errorf("trailing data")
	}
	r, e := fairness.Verify(&a)
	if e != nil {
		return e
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
