// Package service provides import and export functionality for catalog entries.
package service

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/carev01/adhive/internal/model"
)

var csvHeaders = []string{
	"url",
	"title",
	"description",
	"phone_number",
	"location",
}

type ImportRow struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PhoneNumber string `json:"phone_number"`
	Location    string `json:"location"`
}

type RowError struct {
	Row   int    `json:"row"`
	Error string `json:"error"`
}

type SkippedRow struct {
	Row int    `json:"row"`
	URL string `json:"url"`
}

type ImportResult struct {
	ImportedCount int          `json:"imported_count"`
	UpdatedCount  int          `json:"updated_count"`
	SkippedCount  int          `json:"skipped_count"`
	ErrorCount    int          `json:"error_count"`
	SkippedRows   []SkippedRow `json:"skipped_rows,omitempty"`
	Errors        []RowError   `json:"errors,omitempty"`
}

func ParseCSV(reader io.Reader) ([]ImportRow, []RowError, error) {
	csvReader := csv.NewReader(reader)
	headers, err := csvReader.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CSV headers: %w", err)
	}
	normalizedHeaders := make([]string, len(headers))
	for i, h := range headers {
		normalizedHeaders[i] = strings.TrimSpace(strings.ToLower(h))
	}
	headerIndex := make(map[string]int)
	for i, h := range normalizedHeaders {
		headerIndex[h] = i
	}
	if _, ok := headerIndex["url"]; !ok {
		return nil, nil, fmt.Errorf("missing required header: url")
	}
	var rows []ImportRow
	var parseErrors []RowError
	rowNum := 1
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			parseErrors = append(parseErrors, RowError{Row: rowNum, Error: fmt.Sprintf("CSV parse error: %v", err)})
			rowNum++
			continue
		}
		row := ImportRow{}
		if idx, ok := headerIndex["url"]; ok && idx < len(record) {
			row.URL = strings.TrimSpace(record[idx])
		}
		if idx, ok := headerIndex["title"]; ok && idx < len(record) {
			row.Title = strings.TrimSpace(record[idx])
		}
		if idx, ok := headerIndex["description"]; ok && idx < len(record) {
			row.Description = strings.TrimSpace(record[idx])
		}
		if idx, ok := headerIndex["phone_number"]; ok && idx < len(record) {
			row.PhoneNumber = strings.TrimSpace(record[idx])
		}
		if idx, ok := headerIndex["location"]; ok && idx < len(record) {
			row.Location = strings.TrimSpace(record[idx])
		}
		rows = append(rows, row)
		rowNum++
	}
	return rows, parseErrors, nil
}

func ParseJSON(reader io.Reader) ([]ImportRow, []RowError, error) {
	var data struct {
		Entries []ImportRow `json:"entries"`
	}
	if err := json.NewDecoder(reader).Decode(&data); err != nil {
		return nil, nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	if len(data.Entries) == 0 {
		return nil, nil, fmt.Errorf("JSON contains no entries")
	}
	return data.Entries, nil, nil
}

func ValidateRow(row ImportRow, rowNum int) *RowError {
	if row.URL == "" {
		return &RowError{Row: rowNum, Error: "url is required"}
	}
	parsedURL, err := url.Parse(row.URL)
	if err != nil || !parsedURL.IsAbs() || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return &RowError{Row: rowNum, Error: fmt.Sprintf("invalid url: %s", row.URL)}
	}
	if len(row.Title) > 500 {
		return &RowError{Row: rowNum, Error: "title exceeds 500 characters"}
	}
	if len(row.PhoneNumber) > 50 {
		return &RowError{Row: rowNum, Error: "phone_number exceeds 50 characters"}
	}
	if len(row.Location) > 255 {
		return &RowError{Row: rowNum, Error: "location exceeds 255 characters"}
	}
	return nil
}

func RowToEntry(row ImportRow, userID string) *model.CatalogEntry {
	now := time.Now()
	return &model.CatalogEntry{
		ID:            uuid.New().String(),
		UserID:        userID,
		URL:           row.URL,
		Title:         row.Title,
		Description:   row.Description,
		PhoneNumber:   row.PhoneNumber,
		Location:      row.Location,
		ArchiveStatus: model.ArchiveStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func MergeEntry(existing *model.CatalogEntry, row ImportRow) *model.CatalogEntry {
	if row.Title != "" {
		existing.Title = row.Title
	}
	if row.Description != "" {
		existing.Description = row.Description
	}
	if row.PhoneNumber != "" {
		existing.PhoneNumber = row.PhoneNumber
	}
	if row.Location != "" {
		existing.Location = row.Location
	}
	existing.UpdatedAt = time.Now()
	return existing
}

func GenerateCSVTemplate() ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(csvHeaders); err != nil {
		return nil, fmt.Errorf("failed to write CSV headers: %w", err)
	}
	if err := writer.Write([]string{
		"https://example.com/ad/123",
		"Example Ad Title",
		"A description of the ad",
		"555-1234",
		"New York, NY",
	}); err != nil {
		return nil, fmt.Errorf("failed to write example row: %w", err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("CSV write error: %w", err)
	}
	return buf.Bytes(), nil
}

func EntriesToCSV(entries []*model.CatalogEntry) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(csvHeaders); err != nil {
		return nil, fmt.Errorf("failed to write CSV headers: %w", err)
	}
	for _, entry := range entries {
		record := []string{
			entry.URL,
			entry.Title,
			entry.Description,
			entry.PhoneNumber,
			entry.Location,
		}
		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("CSV write error: %w", err)
	}
	return buf.Bytes(), nil
}

func EntriesToJSON(entries []*model.CatalogEntry) ([]byte, error) {
	exportEntries := make([]jsonEntry, len(entries))
	for i, entry := range entries {
		exportEntries[i] = jsonEntry{
			URL:         entry.URL,
			Title:       entry.Title,
			Description: entry.Description,
			PhoneNumber: entry.PhoneNumber,
			Location:    entry.Location,
		}
	}
	data := struct {
		Entries    []jsonEntry `json:"entries"`
		Count      int         `json:"count"`
		ExportedAt string      `json:"exported_at"`
	}{
		Entries:    exportEntries,
		Count:      len(exportEntries),
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
	}
	result, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return result, nil
}

type jsonEntry struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PhoneNumber string `json:"phone_number"`
	Location    string `json:"location"`
}
