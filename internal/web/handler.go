package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jq-view/jq-view/internal/jq"
	"github.com/olekukonko/tablewriter"
)

//go:embed index.html style.css app.js
var staticFiles embed.FS

// serverStartTime is used for hot reload detection
var serverStartTime = time.Now().UnixNano()

// pushHub holds the current dataset and broadcasts revision bumps to SSE
// subscribers. It lets external tools (e.g. an httpYac hook) push a new
// response via POST /api/push and have every open browser refresh in place.
type pushHub struct {
	mu   sync.Mutex
	data []byte
	rev  int
	subs map[chan int]struct{}
}

func newPushHub(initial []byte) *pushHub {
	return &pushHub{data: initial, subs: make(map[chan int]struct{})}
}

// get returns the current dataset and revision.
func (h *pushHub) get() ([]byte, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.data, h.rev
}

// push replaces the dataset, bumps the revision, and notifies subscribers.
func (h *pushHub) push(d []byte) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = d
	h.rev++
	for ch := range h.subs {
		// Non-blocking: a subscriber only needs the latest revision.
		select {
		case ch <- h.rev:
		default:
		}
	}
	return h.rev
}

func (h *pushHub) subscribe() chan int {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan int, 1)
	h.subs[ch] = struct{}{}
	return ch
}

func (h *pushHub) unsubscribe(ch chan int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}

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

	hub := newPushHub(initialData)

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

	// Serve static files (CSS, JS)
	mux.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		data, err := staticFiles.ReadFile("style.css")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write(data)
	})

	mux.HandleFunc("/app.js", func(w http.ResponseWriter, r *http.Request) {
		data, err := staticFiles.ReadFile("app.js")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Write(data)
	})

	// Serve index page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		html, err := staticFiles.ReadFile("index.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Replace placeholder with actual data (latest pushed data, if any)
		curData, _ := hub.get()
		output := strings.Replace(string(html), "{{INITIAL_DATA}}", string(curData), 1)
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

	// API: hot reload + data-push notifications via Server-Sent Events.
	// Two event types are sent:
	//   event: reload  -> data is serverStartTime; client reloads on change (server restart)
	//   event: data    -> data is the push revision; client fetches /api/current
	mux.HandleFunc("/api/reload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Send server start time as reload ID (backwards compatible default event).
		fmt.Fprintf(w, "event: reload\ndata: %d\n\n", serverStartTime)
		flusher.Flush()

		// Subscribe to data pushes and stream revision bumps.
		ch := hub.subscribe()
		defer hub.unsubscribe(ch)

		for {
			select {
			case <-r.Context().Done():
				return
			case rev, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "event: data\ndata: %d\n\n", rev)
				flusher.Flush()
			}
		}
	})

	// API: push a new dataset from an external tool (e.g. httpYac hook).
	// Body is the raw JSON to display. Broadcasts a data event to all clients.
	mux.HandleFunc("/api/push", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", 405)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			respondJSON(w, QueryResponse{Error: err.Error()})
			return
		}

		var js json.RawMessage
		if err := json.Unmarshal(body, &js); err != nil {
			respondJSON(w, QueryResponse{Error: "Invalid JSON: " + err.Error()})
			return
		}

		rev := hub.push(body)
		respondJSON(w, map[string]any{"ok": true, "rev": rev})
	})

	// API: return the current dataset (latest pushed) with its revision.
	mux.HandleFunc("/api/current", func(w http.ResponseWriter, r *http.Request) {
		data, rev := hub.get()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		fmt.Fprintf(w, `{"rev":%d,"data":%s}`, rev, string(data))
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
