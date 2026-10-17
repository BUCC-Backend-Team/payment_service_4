package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

type dotenvEntry struct {
	key   string
	value string
}

func loadDotEnv(path string) error {
	entries, err := parseDotEnvFile(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if _, exists := os.LookupEnv(entry.key); !exists {
			_ = os.Setenv(entry.key, entry.value)
		}
	}

	return nil
}

func parseDotEnvFile(path string) ([]dotenvEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	return parseDotEnv(file)
}

func parseDotEnv(reader io.Reader) ([]dotenvEntry, error) {
	var entries []dotenvEntry

	scanner := bufio.NewScanner(reader)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++

		key, value, err := parseDotEnvLine(scanner.Text())
		if err != nil {
			return nil, fmt.Errorf("config: line %d: %w", lineNumber, err)
		}
		if key == "" {
			continue
		}

		entries = append(entries, dotenvEntry{key: key, value: value})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("config: read .env: %w", err)
	}

	return entries, nil
}

func parseDotEnvLine(line string) (string, string, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", nil
	}

	line = strings.TrimPrefix(line, "export ")

	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", errors.New("expected KEY=VALUE")
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", errors.New("empty variable name")
	}

	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if quote := value[0]; (quote == '"' || quote == '\'') && value[len(value)-1] == quote {
			value = value[1 : len(value)-1]
		}
	}

	return key, value, nil
}
