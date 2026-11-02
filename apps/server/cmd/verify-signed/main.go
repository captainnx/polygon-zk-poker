package main

import (
	"encoding/json"
	"fmt"
	"github.com/polygon-zk-poker/server/internal/table"
	"io"
	"os"
)

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: verify-signed AUDIT.json")
	}
	f, e := os.Open(os.Args[1])
	if e != nil {
		return e
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 16<<20))
	d.DisallowUnknownFields()
	var x table.Snapshot
	if e = d.Decode(&x); e != nil {
		return e
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return fmt.Errorf("trailing data")
	}
	if e = table.Verify(&x); e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"verified": true, "handId": x.Spec.HandID, "actions": len(x.Moves), "finalStacks": x.FinalStacks, "scope": "identity certificates, client key ownership, signed commitments/actions, v3 shuffle and complete engine replay; historical publication time and availability are not proven"})
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
