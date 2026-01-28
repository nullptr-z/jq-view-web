package jq

import (
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
)

// Execute runs a jq expression on input data and returns the result
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

	if len(results) == 1 {
		return json.MarshalIndent(results[0], "", "  ")
	}
	return json.MarshalIndent(results, "", "  ")
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
