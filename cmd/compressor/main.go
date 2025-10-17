package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

var processed sync.Map // track recently processed files to prevent loops

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	log.Printf("Configuration loaded:")
	log.Printf("  Input Dir: %s", cfg.inputDir)
	log.Printf("  Output Dir: %s", cfg.outputDir)
	log.Printf("  FFmpeg Binary: %s", cfg.ffmpegBinary)
	log.Printf("  FFmpeg Command: %s", cfg.ffmpegCommand)
	log.Printf("  Delete Source: %t", cfg.deleteSource)
	log.Printf("  Processing Suffix: %s", cfg.processingSuffix)
	log.Printf("  Output Extension: %s", cfg.outputExtension)
	log.Printf("  HTTP Port: %s", cfg.httpPort)
	log.Printf("  Rescan Interval: %v", cfg.rescanInterval)
	log.Printf("  Stability Window: %v", cfg.stabilityWindow)
	log.Printf("  Queue Size: %d", cfg.queueSize)
	log.Printf("  Max Concurrent: %d", cfg.maxConcurrent)
	var exts []string
	for ext := range cfg.extensions {
		exts = append(exts, ext)
	}
	log.Printf("  Video Extensions: %v", exts)

	if err := os.MkdirAll(cfg.inputDir, 0o755); err != nil {
		log.Fatalf("create input dir: %v", err)
	}
	if err := os.MkdirAll(cfg.outputDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	queue := make(chan string, cfg.queueSize)
	var inProgress sync.Map
	var wg sync.WaitGroup

	dispatcherCtx, dispatcherCancel := context.WithCancel(ctx)
	go func() {
		<-dispatcherCtx.Done()
		close(queue)
	}()

	sem := make(chan struct{}, cfg.maxConcurrent)

	go func() {
		for path := range queue {
			path := path
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer func() {
					<-sem
					inProgress.Delete(path)
					wg.Done()
				}()

				if err := processFile(ctx, cfg, path); err != nil {
					log.Printf("process failed for %s: %v", path, err)
				}
			}()
		}
	}()

	enqueue := func(path string) {
		if !shouldProcess(cfg, path) {
			return
		}
		// Skip if recently processed to prevent loops
		if val, ok := processed.Load(path); ok {
			if recent := val.(time.Time); time.Since(recent) < 10*time.Second {
				return
			}
		}
		if _, loaded := inProgress.LoadOrStore(path, struct{}{}); loaded {
			return
		}
		select {
		case queue <- path:
		default:
			go func() { queue <- path }()
		}
	}

	if err := scanAndEnqueue(cfg, enqueue); err != nil {
		log.Printf("initial scan failed: %v", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("start watcher: %v", err)
	}
	defer watcher.Close()

	if err := watcher.Add(cfg.inputDir); err != nil {
		log.Fatalf("watch dir: %v", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Create|fsnotify.Rename|fsnotify.Write) != 0 {
					enqueue(event.Name)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("watch error: %v", err)
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(cfg.rescanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := scanAndEnqueue(cfg, enqueue); err != nil {
					log.Printf("periodic scan failed: %v", err)
				}
			}
		}
	}()

	serverErrs := make(chan error, 1)
	go func() {
		serverErrs <- runHTTPServer(ctx, cfg.httpPort)
	}()

	select {
	case <-ctx.Done():
	case err := <-serverErrs:
		if err != nil {
			log.Printf("http server error: %v", err)
		}
	}

	dispatcherCancel()
	wg.Wait()
}

func scanAndEnqueue(cfg config, enqueue func(string)) error {
	entries, err := os.ReadDir(cfg.inputDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(cfg.inputDir, entry.Name())
		enqueue(fullPath)
	}
	return nil
}

func shouldProcess(cfg config, path string) bool {
	if !strings.HasPrefix(path, cfg.inputDir) {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !info.Mode().IsRegular() {
		return false
	}
	if strings.HasSuffix(path, cfg.processingSuffix) {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	if _, ok := cfg.extensions[ext]; !ok {
		return false
	}
	return true
}
