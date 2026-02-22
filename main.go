package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ParsedCommand holds a split command and its arguments
type ParsedCommand struct {
	Raw  string   // original full command string
	Cmd  string   // the base command (argv[0])
	Args []string // arguments (argv[1:])
}

// Entry represents a single zsh history entry
type Entry struct {
	Timestamp time.Time
	Duration  int
	Parsed    ParsedCommand
}

// shellSplit splits a shell command string respecting quotes and escapes.
// Handles: single quotes, double quotes, backslash escapes, and unquoted tokens.
func shellSplit(s string) []string {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	i := 0

	for i < len(s) {
		c := rune(s[i])

		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			} else {
				current.WriteRune(c)
			}
			i++

		case inDouble:
			if c == '\\' && i+1 < len(s) {
				next := rune(s[i+1])
				// Only these chars are escapable inside double quotes
				if next == '"' || next == '\\' || next == '$' || next == '`' || next == '\n' {
					current.WriteRune(next)
					i += 2
				} else {
					current.WriteRune(c)
					i++
				}
			} else if c == '"' {
				inDouble = false
				i++
			} else {
				current.WriteRune(c)
				i++
			}

		case c == '\\' && i+1 < len(s):
			// Backslash escape outside quotes
			current.WriteRune(rune(s[i+1]))
			i += 2

		case c == '\'':
			inSingle = true
			i++

		case c == '"':
			inDouble = true
			i++

		case unicode.IsSpace(c):
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			i++

		default:
			current.WriteRune(c)
			i++
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

func parseCommand(raw string) ParsedCommand {
	// Trim leading whitespace and common shell prefixes like `sudo`, `env VAR=val`, etc.
	tokens := shellSplit(strings.TrimSpace(raw))
	if len(tokens) == 0 {
		return ParsedCommand{Raw: raw}
	}

	// Skip past leading env var assignments (e.g. FOO=bar cmd args)
	start := 0
	for start < len(tokens) && strings.Contains(tokens[start], "=") && !strings.HasPrefix(tokens[start], "-") {
		start++
	}
	if start >= len(tokens) {
		// Entire command was env vars
		return ParsedCommand{Raw: raw, Cmd: tokens[0]}
	}

	return ParsedCommand{
		Raw:  raw,
		Cmd:  tokens[start],
		Args: tokens[start+1:],
	}
}

func parseHistory(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var currentCmd strings.Builder

	flushEntry := func(line string) {
		var raw string

		if strings.HasPrefix(line, ": ") {
			parts := strings.SplitN(line, ";", 2)
			if len(parts) == 2 {
				meta := strings.TrimPrefix(parts[0], ": ")
				metaParts := strings.SplitN(meta, ":", 2)
				if len(metaParts) == 2 {
					ts, err1 := strconv.ParseInt(strings.TrimSpace(metaParts[0]), 10, 64)
					dur, err2 := strconv.Atoi(strings.TrimSpace(metaParts[1]))
					if err1 == nil && err2 == nil {
						raw = parts[1]
						entries = append(entries, Entry{
							Timestamp: time.Unix(ts, 0),
							Duration:  dur,
							Parsed:    parseCommand(raw),
						})
						return
					}
				}
			}
		}

		raw = line
		if raw != "" {
			entries = append(entries, Entry{Parsed: parseCommand(raw)})
		}
	}

	for scanner.Scan() {
		line := scanner.Text()

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

	// Example 1: print last 10 entries with split args
	fmt.Println("=== Last 10 Entries ===")
	start := len(entries) - 10
	if start < 0 {
		start = 0
	}
	for _, e := range entries[start:] {
		ts := ""
		if !e.Timestamp.IsZero() {
			ts = fmt.Sprintf("[%s] ", e.Timestamp.Format("2006-01-02 15:04:05"))
		}
		fmt.Printf("%scmd=%q args=%v\n", ts, e.Parsed.Cmd, e.Parsed.Args)
	}

	// Example 2: top 10 most-used base commands
	fmt.Println("\n=== Top 10 Commands ===")
	counts := make(map[string]int)
	for _, e := range entries {
		if e.Parsed.Cmd != "" {
			counts[e.Parsed.Cmd]++
		}
	}
	type kv struct {
		Key   string
		Value int
	}
	var sorted []kv
	for k, v := range counts {
		sorted = append(sorted, kv{k, v})
	}
	// Simple selection sort for top 10
	for i := 0; i < len(sorted) && i < 10; i++ {
		maxIdx := i
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Value > sorted[maxIdx].Value {
				maxIdx = j
			}
		}
		sorted[i], sorted[maxIdx] = sorted[maxIdx], sorted[i]
		fmt.Printf("  %3d  %s\n", sorted[i].Value, sorted[i].Key)
	}
}
