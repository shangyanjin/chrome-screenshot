// Command screenshot is a chromedp example demonstrating how to take a
// screenshot of a specific element and of the entire browser viewport.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

const (
	// Default number of concurrent workers
	defaultThreads = 3
	// JPEG quality for screenshots (1-100)
	defaultQuality = 85
	// Default viewport width in pixels
	defaultViewWidth = 1920
	// Default viewport height in pixels
	defaultViewHeight = 1080
	// Wait time for page resources to load
	pageWaitTime = 3 * time.Second
)

// Screenshot task structure represents a single screenshot job
type screenshotTask struct {
	// Full path to the HTML file
	path string
	// Relative path from input directory
	relPath string
	// Full path where the screenshot will be saved
	outputPath string
}

func main() {
	// Define flags
	dirFlag := flag.String("d", "", "input directory")
	numFlag := flag.Int("n", 3, "number of workers")
	flag.Usage = printUsage
	flag.Parse()

	var inputDir string
	var numWorkers int = 3

	// Check if using flags
	if *dirFlag != "" {
		inputDir = *dirFlag
		numWorkers = *numFlag
	} else {
		// Check position arguments
		args := flag.Args()
		if len(args) < 1 {
			printUsage()
		}
		inputDir = args[0]

		// Parse thread number if provided
		if len(args) > 1 {
			threads, err := strconv.Atoi(args[1])
			if err != nil {
				log.Fatalf("Invalid thread number: %s", args[1])
			}
			if threads < 1 {
				log.Fatalf("Thread number must be greater than 0")
			}
			numWorkers = threads
		}
	}

	// Validate input directory
	if _, err := os.Stat(inputDir); os.IsNotExist(err) {
		log.Fatalf("Input directory %s does not exist", inputDir)
	}

	// Create main context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Process directory and its subdirectories
	if err := dirScreenshot(ctx, inputDir, numWorkers); err != nil {
		log.Fatal(err)
	}

	log.Printf("Screenshot process completed using %d workers", numWorkers)
}

// Worker function to process screenshots in parallel
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

// dirScreenshot processes all HTML files in the input directory and its subdirectories
// using the specified number of concurrent workers
func dirScreenshot(ctx context.Context, inputDir string, numWorkers int) error {
	// Count total HTML files first
	var totalFiles int
	filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".html") {
			totalFiles++
		}
		return nil
	})

	log.Printf("Found %d HTML files in %s", totalFiles, inputDir)

	// Create task channel and wait group
	taskCh := make(chan screenshotTask)
	var wg sync.WaitGroup
	processedFiles := 0
	var processedMutex sync.Mutex

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		workerCtx, _ := chromedp.NewContext(ctx)
		go func(id int) {
			defer wg.Done()
			for task := range taskCh {
				log.Printf("[Worker %d] Processing: %s", id, task.relPath)

				var buf []byte
				fileURL := "file:///" + strings.ReplaceAll(task.path, "\\", "/")

				if err := chromedp.Run(workerCtx, fullScreenshot(fileURL, 90, &buf)); err != nil {
					log.Printf("[Worker %d] Error processing %s: %v", id, task.path, err)
					continue
				}

				if err := os.WriteFile(task.outputPath, buf, 0644); err != nil {
					log.Printf("[Worker %d] Error saving %s: %v", id, task.path, err)
					continue
				}

				processedMutex.Lock()
				processedFiles++
				progress := float64(processedFiles) / float64(totalFiles) * 100
				log.Printf("[Progress: %.1f%%] Completed %s (%d/%d)",
					progress, task.relPath, processedFiles, totalFiles)
				processedMutex.Unlock()
			}
		}(i + 1)
	}

	// Walk through directory and send tasks
	go func() {
		defer close(taskCh)
		currentDir := ""
		dirFileCount := 0

		filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Print directory info
			if info.IsDir() {
				if currentDir != "" {
					log.Printf("Directory completed: %s (%d files)", currentDir, dirFileCount)
				}
				currentDir = path
				dirFileCount = 0
				log.Printf("Entering directory: %s", path)
				return nil
			}

			// Skip if not HTML file
			if !strings.HasSuffix(strings.ToLower(info.Name()), ".html") {
				return nil
			}

			dirFileCount++
			relPath, err := filepath.Rel(inputDir, path)
			if err != nil {
				return err
			}

			outputPath := strings.TrimSuffix(path, ".html") + ".jpg"

			taskCh <- screenshotTask{
				path:       path,
				relPath:    relPath,
				outputPath: outputPath,
			}
			return nil
		})

		// Print last directory info
		if currentDir != "" {
			log.Printf("Directory completed: %s (%d files)", currentDir, dirFileCount)
		}
	}()

	// Wait for all workers to complete
	wg.Wait()
	log.Printf("All files processed. Total: %d files", totalFiles)
	return nil
}

// Print usage information when invalid arguments are provided
func printUsage() {
	fmt.Printf("Usage: %s <input_directory> [thread_number]\n", os.Args[0])
	fmt.Printf("   or: %s -d <input_directory> -n <thread_number>\n\n", os.Args[0])
	fmt.Printf("Example:\n")
	fmt.Printf("  %s ./html_files 5\n", os.Args[0])
	os.Exit(1)
}

// elementScreenshot takes a screenshot of a specific element.
func elementScreenshot(urlstr, sel string, res *[]byte) chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Navigate(urlstr),
		chromedp.Screenshot(sel, res, chromedp.NodeVisible),
	}
}

// fullScreenshot takes a screenshot of the entire browser viewport
// urlstr: URL of the page to screenshot
// quality: JPEG quality (1-100)
// res: pointer to byte slice where the screenshot will be stored
func fullScreenshot(urlstr string, quality int, res *[]byte) chromedp.Tasks {
	return chromedp.Tasks{
		// Set viewport to Full HD resolution
		chromedp.EmulateViewport(defaultViewWidth, defaultViewHeight),
		// Navigate to the target URL
		chromedp.Navigate(urlstr),
		// Wait for the page body to be ready
		chromedp.WaitReady("body", chromedp.ByQuery),
		// Additional wait time for dynamic content and resources
		chromedp.Sleep(pageWaitTime),
		// Take full screenshot with specified quality
		chromedp.FullScreenshot(res, defaultQuality),
	}
}
