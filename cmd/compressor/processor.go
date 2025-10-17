package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/mattn/go-shellwords"
)

func processFile(ctx context.Context, cfg config, originalPath string) error {
	// Get original file size for Discord notifications
	originalInfo, err := os.Stat(originalPath)
	if err != nil {
		return fmt.Errorf("stat original file: %w", err)
	}
	originalSize := originalInfo.Size()

	if err := waitForStability(ctx, originalPath, cfg.stabilityWindow); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		sendDiscordFailure(cfg.discordWebhookURL, originalPath, fmt.Sprintf("stability check: %v", err))
		return fmt.Errorf("stability check: %w", err)
	}

	processingPath := originalPath + cfg.processingSuffix
	if err := os.Rename(originalPath, processingPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		sendDiscordFailure(cfg.discordWebhookURL, originalPath, fmt.Sprintf("rename for processing: %v", err))
		return fmt.Errorf("rename for processing: %w", err)
	}

	success := false
	defer func() {
		if success {
			if cfg.deleteSource {
				if err := os.Remove(processingPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					log.Printf("cleanup remove failed for %s: %v", processingPath, err)
				}
			} else {
				if err := os.Rename(processingPath, originalPath); err != nil {
					log.Printf("restore original failed for %s: %v", processingPath, err)
				}
			}
		} else {
			if _, err := os.Stat(processingPath); err == nil {
				if err := os.Rename(processingPath, originalPath); err != nil {
					log.Printf("restore after failure failed for %s: %v", processingPath, err)
				}
			}
		}
	}()

	outputPath, err := buildOutputPath(cfg, originalPath)
	if err != nil {
		sendDiscordFailure(cfg.discordWebhookURL, originalPath, fmt.Sprintf("build output path: %v", err))
		return err
	}

	if err := runFFMPEG(ctx, cfg, processingPath, outputPath); err != nil {
		sendDiscordFailure(cfg.discordWebhookURL, originalPath, err.Error())
		if removeErr := os.Remove(outputPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			log.Printf("remove partial output %s failed: %v", outputPath, removeErr)
		}
		return err
	}

	success = true
	log.Printf("processed %s -> %s", originalPath, outputPath)

	// Send Discord success notification
	if compressedInfo, err := os.Stat(outputPath); err == nil {
		compressedSize := compressedInfo.Size()

		// Generate thumbnail for Discord webhook
		thumbnailPath := ""
		if cfg.discordWebhookURL != "" {
			// Generate unique thumbnail path in /tmp
			baseName := filepath.Base(strings.TrimSuffix(originalPath, filepath.Ext(originalPath)))
			thumbnailPath = fmt.Sprintf("/tmp/compressor_thumb_%s_%d.jpg", baseName, time.Now().UnixNano())
			if thumbErr := generateThumbnail(ctx, cfg, outputPath, thumbnailPath); thumbErr != nil {
				log.Printf("Failed to generate thumbnail: %v", thumbErr)
				thumbnailPath = "" // Continue without thumbnail
			}
		}

		sendDiscordSuccessWithThumbnail(cfg.discordWebhookURL, originalPath, originalSize, compressedSize, thumbnailPath)

		// Clean up thumbnail file
		if thumbnailPath != "" {
			if err := os.Remove(thumbnailPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				log.Printf("Failed to clean up thumbnail %s: %v", thumbnailPath, err)
			}
		}
	}

	processed.Store(originalPath, time.Now())
	return nil
}

func waitForStability(ctx context.Context, path string, stableFor time.Duration) error {
	if stableFor <= 0 {
		return nil
	}

	const tick = 500 * time.Millisecond
	var (
		prevSize   int64 = -1
		stableTime time.Time
	)

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%s is not a regular file", path)
			}
			size := info.Size()
			if size == prevSize {
				if stableTime.IsZero() {
					stableTime = time.Now()
				}
				if time.Since(stableTime) >= stableFor {
					return nil
				}
			} else {
				prevSize = size
				stableTime = time.Time{}
			}
		}
	}
}

func buildOutputPath(cfg config, originalPath string) (string, error) {
	base := strings.TrimSuffix(filepath.Base(originalPath), filepath.Ext(originalPath))
	ext := cfg.outputExtension
	if ext == "" {
		ext = filepath.Ext(originalPath)
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	candidate := filepath.Join(cfg.outputDir, base+ext)
	if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
		return candidate, nil
	}
	if err := os.MkdirAll(cfg.outputDir, 0o755); err != nil {
		return "", fmt.Errorf("ensure output dir: %w", err)
	}
	for idx := 1; idx < 10_000; idx++ {
		candidate = filepath.Join(cfg.outputDir, fmt.Sprintf("%s_%d%s", base, idx, ext))
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unable to find free output name for %s", originalPath)
}

func runFFMPEG(ctx context.Context, cfg config, inputPath, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("prepare output dir: %w", err)
	}

	substituted := strings.ReplaceAll(cfg.ffmpegCommand, "{{input}}", shellescape.Quote(inputPath))
	substituted = strings.ReplaceAll(substituted, "{{output}}", shellescape.Quote(outputPath))

	args, err := shellwords.Parse(substituted)
	if err != nil {
		return fmt.Errorf("parse ffmpeg args: %w", err)
	}

	cmd := exec.CommandContext(ctx, cfg.ffmpegBinary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	log.Printf("ffmpeg start: %s -> %s", inputPath, outputPath)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w", err)
	}
	return nil
}

func generateThumbnail(ctx context.Context, cfg config, videoPath, thumbnailPath string) error {
	if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0o755); err != nil {
		return fmt.Errorf("prepare thumbnail dir: %w", err)
	}

	// First, probe the video duration
	duration, err := getVideoDuration(ctx, cfg, videoPath)
	if err != nil {
		log.Printf("Failed to get video duration, using default seek: %v", err)
		duration = 10 // fallback to 10 seconds
	}

	// Seek to 10% of the video duration, but at least 1 second
	seekTime := math.Max(1.0, duration*0.1)
	seekString := fmt.Sprintf("%.3f", seekTime)

	// Generate thumbnail at calculated position, full resolution
	args := []string{
		"-skip_frame", "nokey", // Skip non-key frames for faster seeking
		"-i", videoPath,
		"-ss", seekString, // Seek to calculated position
		"-vframes", "1", // Extract 1 frame
		"-y", // Overwrite output
		thumbnailPath,
	}

	cmd := exec.CommandContext(ctx, cfg.ffmpegBinary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	log.Printf("generating thumbnail: %s -> %s (seek to %.1fs)", videoPath, thumbnailPath, seekTime)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("thumbnail generation failed: %w", err)
	}
	return nil
}

func getVideoDuration(ctx context.Context, cfg config, videoPath string) (float64, error) {
	// Use ffprobe to get video duration
	// Assume ffprobe is available alongside ffmpeg
	ffprobePath := strings.Replace(cfg.ffmpegBinary, "ffmpeg", "ffprobe", 1)

	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		videoPath,
	}

	cmd := exec.CommandContext(ctx, ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	// Parse the JSON output to extract duration
	var probeResult struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}

	if err := json.Unmarshal(output, &probeResult); err != nil {
		return 0, fmt.Errorf("parse ffprobe output: %w", err)
	}

	duration, err := strconv.ParseFloat(probeResult.Format.Duration, 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}

	return duration, nil
}
