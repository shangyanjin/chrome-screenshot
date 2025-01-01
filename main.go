// Command screenshot is a chromedp example demonstrating how to take a
// screenshot of a specific element and of the entire browser viewport.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

const (
	// Default input directory
	defaultInputDir = "d:/ui"
	// Number of worker threads
	numWorkers = 3
)

// Screenshot task structure
type screenshotTask struct {
	path       string
	relPath    string
	outputPath string
}

// Worker function to process screenshots
func screenshotWorker(ctx context.Context, taskCh <-chan screenshotTask, wg *sync.WaitGroup) {
	defer wg.Done()

	for task := range taskCh {
		var buf []byte
		fileURL := "file:///" + strings.ReplaceAll(task.path, "\\", "/")

		// Take screenshot
		if err := chromedp.Run(ctx, fullScreenshot(fileURL, 90, &buf)); err != nil {
			log.Printf("Error processing %s: %v", task.path, err)
			continue
		}

		// Save screenshot
		if err := os.WriteFile(task.outputPath, buf, 0644); err != nil {
			log.Printf("Error saving screenshot for %s: %v", task.path, err)
			continue
		}

		log.Printf("Created screenshot for %s", task.relPath)
	}
}

// dirScreenshot processes all HTML files in the default directory and its subdirectories
func dirScreenshot(ctx context.Context) error {
	// Create task channel and wait group
	taskCh := make(chan screenshotTask)
	var wg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		workerCtx, _ := chromedp.NewContext(ctx)
		go screenshotWorker(workerCtx, taskCh, &wg)
	}

	// Walk through directory and send tasks
	go func() {
		defer close(taskCh)
		filepath.Walk(defaultInputDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip if not HTML file
			if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".html") {
				return nil
			}

			// Get relative path for output
			relPath, err := filepath.Rel(defaultInputDir, path)
			if err != nil {
				return err
			}

			// Create output path in the same directory with .jpg extension
			outputPath := strings.TrimSuffix(path, ".html") + ".jpg"

			// Send task to worker
			taskCh <- screenshotTask{
				path:       path,
				relPath:    relPath,
				outputPath: outputPath,
			}
			return nil
		})
	}()

	// Wait for all workers to complete
	wg.Wait()
	return nil
}

func main() {
	// Check if default input directory exists
	if _, err := os.Stat(defaultInputDir); os.IsNotExist(err) {
		log.Fatalf("Input directory %s does not exist", defaultInputDir)
	}

	// Create main context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Process default directory and its subdirectories
	if err := dirScreenshot(ctx); err != nil {
		log.Fatal(err)
	}

	log.Printf("Screenshot process completed")
}

// elementScreenshot takes a screenshot of a specific element.
func elementScreenshot(urlstr, sel string, res *[]byte) chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Navigate(urlstr),
		chromedp.Screenshot(sel, res, chromedp.NodeVisible),
	}
}

// fullScreenshot takes a screenshot of the entire browser viewport.
//
// Note: chromedp.FullScreenshot overrides the device's emulation settings. Use
// device.Reset to reset the emulation and viewport settings.
func fullScreenshot(urlstr string, quality int, res *[]byte) chromedp.Tasks {
	return chromedp.Tasks{
		// Set viewport to Full HD resolution (1920x1080)
		chromedp.EmulateViewport(1920, 1080),
		chromedp.Navigate(urlstr),
		// Wait for the page to be loaded
		chromedp.WaitReady("body", chromedp.ByQuery),
		// Wait 3 seconds to ensure all resources are loaded
		chromedp.Sleep(3 * time.Second),
		// Take full screenshot with 85% quality
		chromedp.FullScreenshot(res, 85),
	}
}
