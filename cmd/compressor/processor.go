package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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
		sendDiscordSuccess(cfg.discordWebhookURL, originalPath, originalSize, compressedSize)
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
