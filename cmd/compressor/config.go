package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultInputDir          = "/input"
	defaultOutputDir         = "/output"
	defaultProcessingSuffix  = ".processing"
	defaultOutputExtension   = ".mp4"
	defaultQueueSize         = 128
	defaultMaxConcurrent     = 3
	defaultHTTPPort          = "8080"
	defaultRescanInterval    = 30 * time.Second
	defaultStabilityDuration = 3 * time.Second
)

const defaultFFMPEGCommand = "-y -hide_banner -nostats -hwaccel cuda -hwaccel_device 0 -i {{input}} -c:v hevc_nvenc -vf format=nv12 -qp 25 -preset p6 -gpu 0 -b_qfactor 1.1 -b_ref_mode middle -bf 3 -g 250 -i_qfactor 0.75 -max_muxing_queue_size 1024 -multipass 1 -rc vbr -rc-lookahead 20 -temporal-aq 1 -tune hq -c:a aac -af volume=2.0 {{output}}"

const defaultFFMPEGCommandCPU = "-y -hide_banner -nostats -i {{input}} -c:v libx265 -preset slow -crf 24 -c:a aac -af volume=2.0 {{output}}"

var defaultExtensions = []string{".mp4", ".mkv", ".mov", ".avi", ".flv", ".wmv", ".m4v", ".webm", ".ts"}

type config struct {
	inputDir          string
	outputDir         string
	ffmpegBinary      string
	ffmpegCommand     string
	deleteSource      bool
	processingSuffix  string
	outputExtension   string
	httpPort          string
	discordWebhookURL string
	rescanInterval    time.Duration
	stabilityWindow   time.Duration
	queueSize         int
	maxConcurrent     int
	extensions        map[string]struct{}
}

func loadConfig() (config, error) {
	cfg := config{
		inputDir:          getEnv("INPUT_DIR", defaultInputDir),
		outputDir:         getEnv("OUTPUT_DIR", defaultOutputDir),
		ffmpegBinary:      getEnv("FFMPEG_BIN", "ffmpeg"),
		processingSuffix:  getEnv("PROCESSING_SUFFIX", defaultProcessingSuffix),
		outputExtension:   getEnv("OUTPUT_EXTENSION", defaultOutputExtension),
		httpPort:          getEnvOrEmpty("PORT"),
		discordWebhookURL: getEnvOrEmpty("DISCORD_WEBHOOK_URL"),
		queueSize:         getEnvInt("QUEUE_SIZE", defaultQueueSize),
		maxConcurrent:     getEnvInt("MAX_CONCURRENT", defaultMaxConcurrent),
		rescanInterval:    getEnvDuration("RESCAN_INTERVAL", defaultRescanInterval),
		stabilityWindow:   getEnvDuration("FILE_STABILITY_DURATION", defaultStabilityDuration),
		deleteSource:      getEnvBool("DELETE_SOURCE"),
	}

	if cfg.maxConcurrent < 1 {
		cfg.maxConcurrent = 1
	}
	if cfg.queueSize < cfg.maxConcurrent {
		cfg.queueSize = cfg.maxConcurrent * 2
	}
	if cfg.processingSuffix == "" {
		cfg.processingSuffix = defaultProcessingSuffix
	}

	extEnv := os.Getenv("VIDEO_EXTENSIONS")
	if strings.TrimSpace(extEnv) == "" {
		extEnv = strings.Join(defaultExtensions, ",")
	}
	cfg.extensions = make(map[string]struct{})
	for _, raw := range strings.Split(extEnv, ",") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, ".") {
			trimmed = "." + trimmed
		}
		cfg.extensions[strings.ToLower(trimmed)] = struct{}{}
	}
	if len(cfg.extensions) == 0 {
		return cfg, errors.New("no video extensions configured")
	}

	// Detect GPU and set ffmpeg command
	gpuAvailable := detectGPU()
	if gpuAvailable {
		cfg.ffmpegCommand = getEnv("FFMPEG_COMMAND", defaultFFMPEGCommand)
	} else {
		log.Printf("GPU not detected, falling back to CPU encoding")
		cfg.ffmpegCommand = getEnv("FFMPEG_COMMAND_CPU", defaultFFMPEGCommandCPU)
		if cfg.ffmpegCommand == "" {
			return cfg, errors.New("GPU not available and no CPU ffmpeg command configured")
		}
	}

	// If inputDir is customized but outputDir is default, assume local testing and set outputDir relative to inputDir
	if cfg.outputDir == defaultOutputDir && cfg.inputDir != defaultInputDir {
		cfg.outputDir = filepath.Join(filepath.Dir(cfg.inputDir), "test_output")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}

func getEnvOrEmpty(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func getEnvBool(key string) bool {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return false
	}
	switch strings.ToLower(val) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func getEnvInt(key string, fallback int) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(val, "%d", &parsed); err != nil {
		log.Printf("invalid int for %s: %v", key, err)
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		log.Printf("invalid duration for %s: %v", key, err)
		return fallback
	}
	return d
}

func detectGPU() bool {
	cmd := exec.Command("nvidia-smi")
	err := cmd.Run()
	return err == nil
}
