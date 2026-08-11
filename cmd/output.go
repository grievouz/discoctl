package cmd

import (
	"encoding/json"
	"io"
)

const schemaVersion = "1"

type envelope struct {
	SchemaVersion string    `json:"schema_version"`
	Data          any       `json:"data"`
	Page          any       `json:"page,omitempty"`
	Warnings      []warning `json:"warnings,omitempty"`
}

type warning struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	ChannelID string `json:"channel_id,omitempty"`
	Count     int    `json:"count,omitempty"`
}

func writeJSON(w io.Writer, data, page any, warnings []warning) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(envelope{
		SchemaVersion: schemaVersion,
		Data:          data,
		Page:          page,
		Warnings:      warnings,
	})
}
