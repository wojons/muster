//go:build wasm
// +build wasm

package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	// Read JSON from stdin — could be object, array, or primitive
	var data interface{}
	if err := json.NewDecoder(os.Stdin).Decode(&data); err != nil {
		fmt.Fprint(os.Stdout, `{"error":"invalid json input"}`)
		os.Exit(1)
	}

	// Inject wasm flag — wrap in object if needed
	switch v := data.(type) {
	case map[string]interface{}:
		v["wasm"] = true
		data = v
	case []interface{}:
		data = map[string]interface{}{"wasm": true, "data": v}
	default:
		data = map[string]interface{}{"wasm": true, "value": v}
	}

	// Write to stdout
	if err := json.NewEncoder(os.Stdout).Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "encode error: %v\n", err)
		os.Exit(1)
	}
}

// Build with:
//   GOOS=wasip1 GOARCH=wasm go build -o response-transform.wasm transform.go
