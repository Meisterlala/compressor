package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"
)

type DiscordWebhook struct {
	URL string
}

type DiscordEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Color       int                 `json:"color"`
	Fields      []DiscordEmbedField `json:"fields"`
	Timestamp   string              `json:"timestamp"`
}

type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type DiscordMessage struct {
	Embeds []DiscordEmbed `json:"embeds"`
}

func sendDiscordSuccess(webhookURL, fileName string, originalSize, compressedSize int64) {
	if webhookURL == "" {
		return
	}

	embed := DiscordEmbed{
		Title:       "✅ Compression Successful",
		Description: fmt.Sprintf("compressed: **%s**", filepath.Base(fileName)),
		Color:       0x00ff00, // Green
		Fields: []DiscordEmbedField{
			{
				Name:   "Original Size",
				Value:  formatFileSize(originalSize),
				Inline: true,
			},
			{
				Name:   "Compressed Size",
				Value:  formatFileSize(compressedSize),
				Inline: true,
			},
			{
				Name:   "Space Saved",
				Value:  formatFileSize(originalSize - compressedSize),
				Inline: true,
			},
			{
				Name:   "Compression Ratio",
				Value:  fmt.Sprintf("%.1f%%", float64(compressedSize)/float64(originalSize)*100),
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	sendDiscordMessage(webhookURL, embed)
}

func sendDiscordFailure(webhookURL, fileName, errorMsg string) {
	if webhookURL == "" {
		return
	}

	embed := DiscordEmbed{
		Title:       "❌ Compression Failed",
		Description: fmt.Sprintf("Failed to compress **%s**", filepath.Base(fileName)),
		Color:       0xff0000, // Red
		Fields: []DiscordEmbedField{
			{
				Name:   "Error",
				Value:  errorMsg,
				Inline: false,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	sendDiscordMessage(webhookURL, embed)
}

func sendDiscordMessage(webhookURL string, embed DiscordEmbed) {
	message := DiscordMessage{
		Embeds: []DiscordEmbed{embed},
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal Discord message: %v", err)
		return
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Failed to send Discord webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		log.Printf("Discord webhook returned status %d", resp.StatusCode)
	}
}

func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
