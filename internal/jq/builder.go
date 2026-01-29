package jq

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
)

// Execute runs a jq expression on input data and returns the result
// The fieldOrder parameter specifies the desired order of fields in the output
func Execute(expression string, input []byte) ([]byte, error) {
	query, err := gojq.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	var inputData any
	if err := json.Unmarshal(input, &inputData); err != nil {
		return nil, fmt.Errorf("json error: %w", err)
	}

	var results []any
	iter := query.Run(inputData)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return nil, err
		}
		results = append(results, v)
	}

	// Extract field order from expression
	fieldOrder := extractFieldOrder(expression)

	if len(results) == 1 {
		return marshalOrdered(results[0], fieldOrder)
	}
	return marshalOrdered(results, fieldOrder)
}

// extractFieldOrder extracts field names from jq expression in order
func extractFieldOrder(expr string) []string {
	var fields []string

	// Match patterns like {field1, field2} or {field1: .path, field2: .path}
	// Find content between { and }
	start := strings.LastIndex(expr, "{")
	end := strings.LastIndex(expr, "}")
	if start == -1 || end == -1 || start >= end {
		return nil
	}

	content := expr[start+1 : end]
	parts := strings.Split(content, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// Handle "field: .path" or just "field"
		if colonIdx := strings.Index(p, ":"); colonIdx != -1 {
			field := strings.TrimSpace(p[:colonIdx])
			fields = append(fields, field)
		} else {
			fields = append(fields, p)
		}
	}

	return fields
}

// marshalOrdered marshals JSON with fields in specified order
func marshalOrdered(data any, fieldOrder []string) ([]byte, error) {
	if len(fieldOrder) == 0 {
		return json.MarshalIndent(data, "", "  ")
	}

	var buf bytes.Buffer
	if err := writeOrderedJSON(&buf, data, fieldOrder, 0); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeOrderedJSON(buf *bytes.Buffer, data any, fieldOrder []string, indent int) error {
	indentStr := strings.Repeat("  ", indent)
	nextIndent := strings.Repeat("  ", indent+1)

	switch v := data.(type) {
	case map[string]any:
		buf.WriteString("{\n")

		// Order keys: first by fieldOrder, then alphabetically for remaining
		orderedKeys := orderKeys(v, fieldOrder)

		for i, k := range orderedKeys {
			if i > 0 {
				buf.WriteString(",\n")
			}
			buf.WriteString(nextIndent)
			buf.WriteString(fmt.Sprintf("%q: ", k))
			if err := writeOrderedJSON(buf, v[k], fieldOrder, indent+1); err != nil {
				return err
			}
		}
		buf.WriteString("\n")
		buf.WriteString(indentStr)
		buf.WriteString("}")

	case []any:
		buf.WriteString("[\n")
		for i, item := range v {
			if i > 0 {
				buf.WriteString(",\n")
			}
			buf.WriteString(nextIndent)
			if err := writeOrderedJSON(buf, item, fieldOrder, indent+1); err != nil {
				return err
			}
		}
		buf.WriteString("\n")
		buf.WriteString(indentStr)
		buf.WriteString("]")

	default:
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		buf.Write(b)
	}

	return nil
}

func orderKeys(m map[string]any, fieldOrder []string) []string {
	// Create a map of field positions
	orderMap := make(map[string]int)
	for i, f := range fieldOrder {
		orderMap[f] = i
	}

	// Collect all keys
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	// Sort by order: fields in fieldOrder first (by position), then alphabetically
	sort.Slice(keys, func(i, j int) bool {
		posI, hasI := orderMap[keys[i]]
		posJ, hasJ := orderMap[keys[j]]

		if hasI && hasJ {
			return posI < posJ
		}
		if hasI {
			return true
		}
		if hasJ {
			return false
		}
		return keys[i] < keys[j]
	})

	return keys
}

// BuildExpressionFromFields generates a jq expression from field names
func BuildExpressionFromFields(fields []string, isArray bool) string {
	if len(fields) == 0 {
		return "."
	}

	if len(fields) == 1 {
		if isArray {
			return fmt.Sprintf(".[] | .%s", fields[0])
		}
		return "." + fields[0]
	}

	// Multiple fields: create object
	expr := "{"
	for i, f := range fields {
		if i > 0 {
			expr += ", "
		}
		expr += f
	}
	expr += "}"

	if isArray {
		return ".[] | " + expr
	}
	return expr
}
