package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// renderJSON marshals v to w as indented JSON.
func renderJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// renderKV prints a key=value pair line.
func renderKV(w io.Writer, key string, val any) {
	fmt.Fprintf(w, "%-14s %v\n", key+":", val)
}
