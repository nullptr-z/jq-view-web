package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/jq-view/jq-view/internal/web"
)

func main() {
	port := flag.Int("p", 8080, "Port to listen on")
	noBrowser := flag.Bool("no-browser", false, "Don't open browser automatically")
	flag.Parse()

	var data []byte
	var err error

	// Read from file or stdin
	args := flag.Args()
	if len(args) > 0 {
		data, err = os.ReadFile(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Check if stdin has data
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err = io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Usage: jq-view [file.json]")
			fmt.Fprintln(os.Stderr, "       cat file.json | jq-view")
			fmt.Fprintln(os.Stderr, "\nOptions:")
			fmt.Fprintln(os.Stderr, "  -p PORT        Port to listen on (default 8080)")
			fmt.Fprintln(os.Stderr, "  -no-browser    Don't open browser automatically")
			os.Exit(1)
		}
	}

	if len(data) == 0 {
		fmt.Fprintln(os.Stderr, "No input data")
		os.Exit(1)
	}

	// Validate JSON
	var js json.RawMessage
	if err := json.Unmarshal(data, &js); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid JSON: %v\n", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf(":%d", *port)
	url := fmt.Sprintf("http://localhost:%d", *port)

	fmt.Printf("Starting jq-view at %s\n", url)
	fmt.Println("Press Ctrl+C to stop")

	// Open browser
	if !*noBrowser {
		go openBrowser(url)
	}

	handler := web.Handler(data)
	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		cmd.Start()
	}
}
