package jsonutil

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

func ReadJSONL(path string) ([]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var rows []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, sc.Err()
}

func AppendJSONL(path string, row map[string]any) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	data, err := json.Marshal(row)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

func WriteJSONL(path string, rows []map[string]any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}
