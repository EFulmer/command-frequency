package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Entry represents a single zsh history entry
type Entry struct {
	Timestamp time.Time
	Duration  int
	Command   string
}

func parseHistory(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)

	// Increase buffer size for long lines
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var currentCmd strings.Builder

	flushEntry := func(line string) {
		// Extended history format: ": <timestamp>:<duration>;<command>"
		if strings.HasPrefix(line, ": ") {
			parts := strings.SplitN(line, ";", 2)
			if len(parts) == 2 {
				meta := strings.TrimPrefix(parts[0], ": ")
				metaParts := strings.SplitN(meta, ":", 2)
				if len(metaParts) == 2 {
					ts, err1 := strconv.ParseInt(strings.TrimSpace(metaParts[0]), 10, 64)
					dur, err2 := strconv.Atoi(strings.TrimSpace(metaParts[1]))
					if err1 == nil && err2 == nil {
						entries = append(entries, Entry{
							Timestamp: time.Unix(ts, 0),
							Duration:  dur,
							Command:   parts[1],
						})
						return
					}
				}
			}
		}
		// Plain format (no metadata)
		if line != "" {
			entries = append(entries, Entry{Command: line})
		}
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Handle multi-line commands (lines ending with `\`)
		if strings.HasSuffix(line, "\\") {
			currentCmd.WriteString(strings.TrimSuffix(line, "\\"))
			currentCmd.WriteString("\n")
			continue
		}

		if currentCmd.Len() > 0 {
			currentCmd.WriteString(line)
			flushEntry(currentCmd.String())
			currentCmd.Reset()
		} else {
			flushEntry(line)
		}
	}

	// Flush any remaining command
	if currentCmd.Len() > 0 {
		flushEntry(currentCmd.String())
	}

	return entries, scanner.Err()
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding home dir: %v\n", err)
		os.Exit(1)
	}

	histPath := filepath.Join(home, ".zsh_history")
	entries, err := parseHistory(histPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing history: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Parsed %d history entries\n\n", len(entries))

	// Example: print the last 10 entries
	start := len(entries) - 10
	if start < 0 {
		start = 0
	}
	fmt.Println("Last 10 commands:")
	for _, e := range entries[start:] {
		if !e.Timestamp.IsZero() {
			fmt.Printf("[%s] (dur: %ds) %s\n", e.Timestamp.Format("2006-01-02 15:04:05"), e.Duration, e.Command)
		} else {
			fmt.Printf("%s\n", e.Command)
		}
	}
}
