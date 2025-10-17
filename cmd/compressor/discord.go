package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
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
	Image       *DiscordEmbedImage  `json:"image,omitempty"`
	Timestamp   string              `json:"timestamp"`
}

type DiscordEmbedImage struct {
	URL string `json:"url"`
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
	sendDiscordSuccessWithThumbnail(webhookURL, fileName, originalSize, compressedSize, "")
}

func sendDiscordSuccessWithThumbnail(webhookURL, fileName string, originalSize, compressedSize int64, thumbnailPath string) {
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

	// Add thumbnail image to embed if available
	if thumbnailPath != "" {
		embed.Image = &DiscordEmbedImage{
			URL: "attachment://thumbnail.jpg",
		}
	}

	sendDiscordMessageWithAttachment(webhookURL, embed, thumbnailPath, "thumbnail.jpg")
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
	sendDiscordMessageWithAttachment(webhookURL, embed, "", "")
}

func sendDiscordMessageWithAttachment(webhookURL string, embed DiscordEmbed, attachmentPath, attachmentName string) {
	message := DiscordMessage{
		Embeds: []DiscordEmbed{embed},
	}

	if attachmentPath == "" {
		// Send JSON-only message
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
		return
	}

	// Send message with file attachment using multipart/form-data
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// Add the JSON payload
	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal Discord message: %v", err)
		return
	}

	if err := w.WriteField("payload_json", string(jsonData)); err != nil {
		log.Printf("Failed to write payload_json field: %v", err)
		return
	}

	// Add the file attachment
	file, err := os.Open(attachmentPath)
	if err != nil {
		log.Printf("Failed to open attachment file: %v", err)
		return
	}
	defer file.Close()

	fw, err := w.CreateFormFile("file", attachmentName)
	if err != nil {
		log.Printf("Failed to create form file: %v", err)
		return
	}

	if _, err := io.Copy(fw, file); err != nil {
		log.Printf("Failed to copy file data: %v", err)
		return
	}

	w.Close()

	req, err := http.NewRequest("POST", webhookURL, &b)
	if err != nil {
		log.Printf("Failed to create HTTP request: %v", err)
		return
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to send Discord webhook with attachment: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		log.Printf("Discord webhook with attachment returned status %d", resp.StatusCode)
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
