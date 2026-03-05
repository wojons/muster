package response

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

type ContentParser interface {
	Parse(body []byte) (interface{}, error)
	CanParse(contentType string) bool
}

type JSONParser struct{}

func (p *JSONParser) CanParse(contentType string) bool {
	return strings.Contains(contentType, "json")
}

func (p *JSONParser) Parse(body []byte) (interface{}, error) {
	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err == nil {
		return obj, nil
	}

	var arr []interface{}
	err := json.Unmarshal(body, &arr)
	if err == nil {
		return arr, nil
	}

	return nil, fmt.Errorf("failed to parse JSON: %w", err)
}

type XMLParser struct{}

func (p *XMLParser) CanParse(contentType string) bool {
	return strings.Contains(contentType, "xml")
}

func (p *XMLParser) Parse(body []byte) (interface{}, error) {
	var root map[string]interface{}
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}
	return root, nil
}

type YAMLParser struct{}

func (p *YAMLParser) CanParse(contentType string) bool {
	return strings.Contains(contentType, "yaml") ||
		strings.Contains(contentType, "yml")
}

func (p *YAMLParser) Parse(body []byte) (interface{}, error) {
	var obj map[string]interface{}
	if err := yaml.Unmarshal(body, &obj); err == nil {
		return obj, nil
	}

	var arr []interface{}
	err := yaml.Unmarshal(body, &arr)
	if err == nil {
		return arr, nil
	}

	return nil, fmt.Errorf("failed to parse YAML: %w", err)
}

type CSVParser struct{}

func (p *CSVParser) CanParse(contentType string) bool {
	return strings.Contains(contentType, "csv")
}

func (p *CSVParser) Parse(body []byte) (interface{}, error) {
	reader := csv.NewReader(strings.NewReader(string(body)))

	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV headers: %w", err)
	}

	var records []map[string]string

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CSV row: %w", err)
		}

		record := make(map[string]string)
		for i, header := range headers {
			if i < len(row) {
				record[header] = row[i]
			}
		}
		records = append(records, record)
	}

	return records, nil
}

type TextParser struct{}

func (p *TextParser) CanParse(contentType string) bool {
	return true
}

func (p *TextParser) Parse(body []byte) (interface{}, error) {
	return string(body), nil
}

func NewContentParser(config Config) ContentParser {
	return &AutoParser{}
}

type AutoParser struct{}

func (p *AutoParser) CanParse(contentType string) bool {
	return true
}

func (p *AutoParser) Parse(body []byte) (interface{}, error) {
	var jsonData interface{}
	if err := json.Unmarshal(body, &jsonData); err == nil {
		return jsonData, nil
	}

	var yamlData interface{}
	if err := yaml.Unmarshal(body, &yamlData); err == nil {
		return yamlData, nil
	}

	return string(body), nil
}
