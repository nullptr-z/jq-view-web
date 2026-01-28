package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jq-view/jq-view/internal/jq"
	"github.com/olekukonko/tablewriter"
)

//go:embed index.html
var indexHTML embed.FS

type QueryRequest struct {
	Data       json.RawMessage `json:"data"`
	Expression string          `json:"expression"`
	Format     string          `json:"format"` // json or table
}

type QueryResponse struct {
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

// Handler returns the HTTP handler for the web UI
func Handler(initialData []byte) http.Handler {
	mux := http.NewServeMux()

	// Serve index page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		html, err := indexHTML.ReadFile("index.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Replace placeholder with actual data
		output := strings.Replace(string(html), "{{INITIAL_DATA}}", string(initialData), 1)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(output))
	})

	// API: execute jq query
	mux.HandleFunc("/api/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", 405)
			return
		}

		var req QueryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, QueryResponse{Error: err.Error()})
			return
		}

		result, err := jq.Execute(req.Expression, req.Data)
		if err != nil {
			respondJSON(w, QueryResponse{Error: err.Error()})
			return
		}

		// Convert to table if requested
		if req.Format == "table" {
			tableStr, err := jsonToTable(result)
			if err != nil {
				respondJSON(w, QueryResponse{Result: string(result)})
				return
			}
			respondJSON(w, QueryResponse{Result: tableStr})
			return
		}

		respondJSON(w, QueryResponse{Result: string(result)})
	})

	return mux
}

func respondJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonToTable(data []byte) (string, error) {
	var items []map[string]any

	// Try array first
	if err := json.Unmarshal(data, &items); err != nil {
		// Try single object
		var single map[string]any
		if err := json.Unmarshal(data, &single); err != nil {
			return "", fmt.Errorf("cannot convert to table")
		}
		items = []map[string]any{single}
	}

	if len(items) == 0 {
		return "", fmt.Errorf("empty data")
	}

	// Get headers from first item
	var headers []string
	for k := range items[0] {
		headers = append(headers, k)
	}

	var buf bytes.Buffer
	table := tablewriter.NewTable(&buf)
	table.Header(toAny(headers)...)

	for _, item := range items {
		var row []any
		for _, h := range headers {
			row = append(row, formatValue(item[h]))
		}
		table.Append(row...)
	}

	table.Render()
	return buf.String(), nil
}

func toAny(s []string) []any {
	r := make([]any, len(s))
	for i, v := range s {
		r[i] = v
	}
	return r
}

func formatValue(v any) string {
	switch val := v.(type) {
	case nil:
		return "null"
	case string:
		return val
	case float64:
		if val == float64(int(val)) {
			return fmt.Sprintf("%d", int(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
