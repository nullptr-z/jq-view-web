package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

type FileListResponse struct {
	Files       []string `json:"files"`
	CurrentFile string   `json:"currentFile"`
	DirPath     string   `json:"dirPath"`
}

type LoadFileRequest struct {
	Filename string `json:"filename"`
}

type LoadFileResponse struct {
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// Handler returns the HTTP handler for the web UI
func Handler(initialData []byte, dirPath string) http.Handler {
	mux := http.NewServeMux()

	currentFile := ""
	if dirPath != "" {
		// Find the first JSON file name
		entries, _ := os.ReadDir(dirPath)
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
				currentFile = e.Name()
				break
			}
		}
	}

	// Serve index page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		html, err := indexHTML.ReadFile("index.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Replace placeholder with actual data
		output := strings.Replace(string(html), "{{INITIAL_DATA}}", string(initialData), 1)
		// Replace directory mode flag
		dirModeStr := "false"
		if dirPath != "" {
			dirModeStr = "true"
		}
		output = strings.Replace(output, "{{DIR_MODE}}", dirModeStr, 1)
		output = strings.Replace(output, "{{CURRENT_FILE}}", currentFile, 1)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(output))
	})

	// API: list files in directory
	mux.HandleFunc("/api/files", func(w http.ResponseWriter, r *http.Request) {
		if dirPath == "" {
			respondJSON(w, FileListResponse{Files: nil})
			return
		}

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			respondJSON(w, FileListResponse{Files: nil})
			return
		}

		var files []string
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
				files = append(files, e.Name())
			}
		}

		respondJSON(w, FileListResponse{
			Files:       files,
			CurrentFile: currentFile,
			DirPath:     dirPath,
		})
	})

	// API: load a specific file
	mux.HandleFunc("/api/load", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", 405)
			return
		}

		if dirPath == "" {
			respondJSON(w, LoadFileResponse{Error: "Not in directory mode"})
			return
		}

		var req LoadFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, LoadFileResponse{Error: err.Error()})
			return
		}

		// Security: ensure filename doesn't contain path traversal
		if strings.Contains(req.Filename, "..") || strings.Contains(req.Filename, "/") {
			respondJSON(w, LoadFileResponse{Error: "Invalid filename"})
			return
		}

		filePath := filepath.Join(dirPath, req.Filename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			respondJSON(w, LoadFileResponse{Error: err.Error()})
			return
		}

		// Validate JSON
		var js json.RawMessage
		if err := json.Unmarshal(data, &js); err != nil {
			respondJSON(w, LoadFileResponse{Error: "Invalid JSON: " + err.Error()})
			return
		}

		currentFile = req.Filename
		respondJSON(w, LoadFileResponse{Data: js})
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
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("cannot parse JSON")
	}

	var buf bytes.Buffer
	renderTables(&buf, "", parsed)

	if buf.Len() == 0 {
		return "", fmt.Errorf("no tabular data found")
	}

	return buf.String(), nil
}

// renderTables recursively renders tables for each level of the data
func renderTables(buf *bytes.Buffer, title string, data any) {
	switch v := data.(type) {
	case []any:
		// Array of objects -> render as table
		if len(v) == 0 {
			return
		}
		// Check if first element is an object
		if obj, ok := v[0].(map[string]any); ok {
			renderArrayTable(buf, title, v)
			// Recursively render nested arrays/objects
			for key := range obj {
				var nestedArrays []any
				for _, item := range v {
					if m, ok := item.(map[string]any); ok {
						if nested, exists := m[key]; exists {
							if arr, isArr := nested.([]any); isArr {
								nestedArrays = append(nestedArrays, arr...)
							}
						}
					}
				}
				if len(nestedArrays) > 0 {
					renderTables(buf, key, nestedArrays)
				}
			}
		} else {
			// Array of primitives
			renderPrimitiveArray(buf, title, v)
		}
	case map[string]any:
		// Single object - collect leaf values and nested structures
		leafs := make(map[string]any)
		for key, val := range v {
			switch nested := val.(type) {
			case []any:
				renderTables(buf, key, nested)
			case map[string]any:
				renderTables(buf, key, nested)
			default:
				leafs[key] = val
			}
		}
		// Render leaf values as single-row table
		if len(leafs) > 0 {
			renderObjectTable(buf, title, leafs)
		}
	}
}

// renderArrayTable renders an array of objects as a table
func renderArrayTable(buf *bytes.Buffer, title string, items []any) {
	if len(items) == 0 {
		return
	}

	// Collect all leaf keys (non-object, non-array)
	firstObj, ok := items[0].(map[string]any)
	if !ok {
		return
	}

	var headers []string
	for k, v := range firstObj {
		switch v.(type) {
		case []any, map[string]any:
			// Skip nested structures
		default:
			headers = append(headers, k)
		}
	}

	if len(headers) == 0 {
		return
	}

	// Sort headers for consistent order
	// (keeping insertion order from map iteration)

	if title != "" {
		buf.WriteString(fmt.Sprintf("\n── %s ──\n", title))
	}

	table := tablewriter.NewTable(buf)
	table.Header(toAny(headers)...)

	for _, item := range items {
		if obj, ok := item.(map[string]any); ok {
			var row []any
			for _, h := range headers {
				row = append(row, formatValue(obj[h]))
			}
			table.Append(row...)
		}
	}

	table.Render()
}

// renderObjectTable renders a single object as a table
func renderObjectTable(buf *bytes.Buffer, title string, obj map[string]any) {
	if len(obj) == 0 {
		return
	}

	var headers []string
	var values []any
	for k, v := range obj {
		headers = append(headers, k)
		values = append(values, formatValue(v))
	}

	if title != "" {
		buf.WriteString(fmt.Sprintf("\n── %s ──\n", title))
	}

	table := tablewriter.NewTable(buf)
	table.Header(toAny(headers)...)
	table.Append(values...)
	table.Render()
}

// renderPrimitiveArray renders an array of primitive values
func renderPrimitiveArray(buf *bytes.Buffer, title string, items []any) {
	if len(items) == 0 {
		return
	}

	if title == "" {
		title = "values"
	}

	buf.WriteString(fmt.Sprintf("\n── %s ──\n", title))

	table := tablewriter.NewTable(buf)
	table.Header(title)

	for _, item := range items {
		table.Append(formatValue(item))
	}

	table.Render()
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
