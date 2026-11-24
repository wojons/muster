package asyncapi

import (
	"encoding/json"

	"github.com/wojons/muster/pkg/errors"
	"gopkg.in/yaml.v3"
)

// yamlToMap decodes a YAML document body into the generic map shape JSON
// decoding produces, re-marshalling through JSON bytes so every downstream
// walker sees one representation (the map-bridge pattern pkg/openapi uses
// for Swagger 2.0 documents — yaml.v3 does not drive json.Unmarshal).
func yamlToMap(data []byte) (map[string]interface{}, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	if len(node.Content) == 0 {
		return nil, errors.New(errors.CodeValidation, "asyncapi: YAML document is empty")
	}
	// Re-marshal the YAML tree and decode it as JSON so value types match
	// the JSON path exactly (floats, bools, strings — no yaml.MapSlice).
	bridged, err := yaml.Marshal(&node)
	if err != nil {
		return nil, err
	}
	var generic interface{}
	if err := yaml.Unmarshal(bridged, &generic); err != nil {
		return nil, err
	}
	asJSON, err := json.Marshal(generic)
	if err != nil {
		return nil, err
	}
	var root map[string]interface{}
	if err := json.Unmarshal(asJSON, &root); err != nil {
		return nil, err
	}
	return root, nil
}
