package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// Steam Store API configuration
	steamAPIEndpoint = "https://store.steampowered.com/api/storesearch/"
	apiLanguage      = "english"
	apiCountryCode   = "US"
	apiTimeout       = 10 * time.Second

	// File permissions
	fileMode = 0644
	dirMode  = 0755

	// Display settings
	maxVerboseResults = 3 // Show top N results in verbose mode
	summarySeparator  = 50
)

// SteamAPIResponse represents the JSON response from Steam Store API
type SteamAPIResponse struct {
	Items []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"items"`
}

// GameResult represents the outcome of processing a single game
type GameResult struct {
	GameName string
	GameID   int
	Success  bool
	Error    error
}

// findSteamGameID queries the Steam Store API and returns the game ID for the given name
func findSteamGameID(gameName string, verbose bool) (int, error) {
	if gameName == "" {
		return 0, fmt.Errorf("empty game name")
	}

	// Build API request URL with query parameters
	queryParams := url.Values{}
	queryParams.Set("term", gameName)
	queryParams.Set("l", apiLanguage)
	queryParams.Set("cc", apiCountryCode)
	requestURL := steamAPIEndpoint + "?" + queryParams.Encode()

	if verbose {
		fmt.Fprintf(os.Stderr, "  [DEBUG] Querying: %s\n", requestURL)
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{Timeout: apiTimeout}
	requestStartTime := time.Now()

	// Make API request
	httpResponse, err := httpClient.Get(requestURL)
	if err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "  [ERROR] Network error: %v\n", err)
		}
		return 0, err
	}
	defer httpResponse.Body.Close()

	if verbose {
		requestDuration := time.Since(requestStartTime)
		fmt.Fprintf(os.Stderr, "  [DEBUG] Response status: %d (took %v)\n", httpResponse.StatusCode, requestDuration)
	}

	// Validate HTTP response status
	if httpResponse.StatusCode != http.StatusOK {
		err := fmt.Errorf("HTTP error: %d", httpResponse.StatusCode)
		if verbose {
			fmt.Fprintf(os.Stderr, "  [ERROR] %v\n", err)
		}
		return 0, err
	}

	// Read response body
	responseBody, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "  [ERROR] Failed to read response: %v\n", err)
		}
		return 0, err
	}

	// Parse JSON response
	var apiResponse SteamAPIResponse
	if err := json.Unmarshal(responseBody, &apiResponse); err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "  [ERROR] JSON parse error: %v\n", err)
		}
		return 0, err
	}

	// Log search results in verbose mode
	if verbose {
		resultCount := len(apiResponse.Items)
		fmt.Fprintf(os.Stderr, "  [DEBUG] Found %d result(s)\n", resultCount)
		if resultCount > 0 {
			// Show top results for debugging
			for i, item := range apiResponse.Items {
				if i < maxVerboseResults {
					fmt.Fprintf(os.Stderr, "    %d. %s (ID: %d)\n", i+1, item.Name, item.ID)
				}
			}
		}
	}

	// Return the top-ranked result (Steam's API returns best match first)
	if len(apiResponse.Items) > 0 {
		return apiResponse.Items[0].ID, nil
	}

	// No results found
	err = fmt.Errorf("no results found")
	if verbose {
		fmt.Fprintf(os.Stderr, "  [ERROR] %v\n", err)
	}
	return 0, err
}

// sanitizeFilename converts a game name into a safe filename by removing invalid characters
func sanitizeFilename(gameName string) string {
	// Keep only alphanumeric, spaces, hyphens, and underscores
	var sanitized strings.Builder
	for _, char := range gameName {
		isValidChar := (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == ' ' || char == '-' || char == '_'
		if isValidChar {
			sanitized.WriteRune(char)
		} else {
			sanitized.WriteRune('_') // Replace invalid chars with underscore
		}
	}

	// Collapse multiple spaces/underscores into single underscore
	spaceUnderscorePattern := regexp.MustCompile(`[_\s]+`)
	cleaned := spaceUnderscorePattern.ReplaceAllString(sanitized.String(), "_")

	// Remove leading/trailing underscores
	cleaned = strings.Trim(cleaned, "_")

	return cleaned
}

// processSingleGame handles one game: looks up ID and writes .steam file
func processSingleGame(gameName string, outputDirectory string, shouldSkipExisting bool, verbose bool) GameResult {
	// Generate output file path
	sanitizedFilename := sanitizeFilename(gameName)
	steamFilePath := filepath.Join(outputDirectory, sanitizedFilename+".steam")

	// Skip existing files if requested
	if shouldSkipExisting {
		if _, err := os.Stat(steamFilePath); err == nil {
			// File exists - read existing ID instead of querying API
			if verbose {
				fmt.Fprintf(os.Stderr, "  [SKIP] File already exists: %s\n", steamFilePath)
			}
			existingFileContent, err := os.ReadFile(steamFilePath)
			if err == nil {
				var existingGameID int
				if _, err := fmt.Sscanf(string(existingFileContent), "%d", &existingGameID); err == nil {
					return GameResult{
						GameName: gameName,
						GameID:   existingGameID,
						Success:  true,
					}
				}
			}
		}
	}

	// Query Steam API for game ID
	gameID, err := findSteamGameID(gameName, verbose)
	if err != nil {
		return GameResult{
			GameName: gameName,
			Success:  false,
			Error:    err,
		}
	}

	// Write game ID to .steam file
	gameIDString := fmt.Sprintf("%d", gameID)
	if err := os.WriteFile(steamFilePath, []byte(gameIDString), fileMode); err != nil {
		return GameResult{
			GameName: gameName,
			Success:  false,
			Error:    fmt.Errorf("error writing file: %v", err),
		}
	}

	return GameResult{
		GameName: gameName,
		GameID:   gameID,
		Success:  true,
	}
}

// workerGoroutine processes games from the jobs channel and sends results back
func workerGoroutine(jobQueue <-chan string, resultQueue chan<- GameResult, outputDirectory string, shouldSkipExisting bool, verbose bool, waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()
	for gameName := range jobQueue {
		resultQueue <- processSingleGame(gameName, outputDirectory, shouldSkipExisting, verbose)
	}
}

func main() {
	// Parse command-line flags
	outputDirectoryFlag := flag.String("output", "output", "Output directory for .steam files")
	workerCountFlag := flag.Int("workers", 8, "Number of concurrent workers")
	skipExistingFlag := flag.Bool("skip-existing", false, "Skip games that already have .steam files")
	verboseFlag := flag.Bool("verbose", false, "Show detailed output")

	// Custom help message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <input_file.txt>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s games.txt\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --output my_output --workers 16 --skip-existing games.txt\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --verbose games.txt\n", os.Args[0])
	}

	flag.Parse()

	// Validate input file argument
	commandLineArgs := flag.Args()
	if len(commandLineArgs) < 1 {
		flag.Usage()
		os.Exit(1)
	}
	inputFilePath := commandLineArgs[0]

	// Verify input file exists
	if _, err := os.Stat(inputFilePath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: File '%s' not found.\n", inputFilePath)
		os.Exit(1)
	}

	// Create output directory
	if err := os.MkdirAll(*outputDirectoryFlag, dirMode); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Read and parse input file
	inputFileContent, err := os.ReadFile(inputFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Extract game names from file (one per line, skip empty lines)
	inputLines := strings.Split(string(inputFileContent), "\n")
	var gameNames []string
	for _, line := range inputLines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine != "" {
			gameNames = append(gameNames, trimmedLine)
		}
	}

	if len(gameNames) == 0 {
		fmt.Fprintf(os.Stderr, "Error: No game names found in file.\n")
		os.Exit(1)
	}

	// Display processing info
	totalGames := len(gameNames)
	fmt.Printf("Processing %d game(s)...\n", totalGames)
	if *verboseFlag {
		fmt.Printf("  Output directory: %s\n", *outputDirectoryFlag)
		fmt.Printf("  Workers: %d\n", *workerCountFlag)
		fmt.Printf("  Skip existing: %v\n", *skipExistingFlag)
	}
	fmt.Println()

	// Adjust worker count if we have fewer games than workers
	actualWorkerCount := *workerCountFlag
	if totalGames < actualWorkerCount {
		actualWorkerCount = totalGames
	}

	// Set up channels for parallel processing
	jobQueue := make(chan string, totalGames)
	resultQueue := make(chan GameResult, totalGames)

	// Start worker pool
	var workerWaitGroup sync.WaitGroup
	for i := 0; i < actualWorkerCount; i++ {
		workerWaitGroup.Add(1)
		go workerGoroutine(jobQueue, resultQueue, *outputDirectoryFlag, *skipExistingFlag, *verboseFlag, &workerWaitGroup)
	}

	// Send all game names to job queue
	go func() {
		for _, gameName := range gameNames {
			jobQueue <- gameName
		}
		close(jobQueue)
	}()

	// Close result queue when all workers finish
	go func() {
		workerWaitGroup.Wait()
		close(resultQueue)
	}()

	// Collect and display results as they complete
	successCount := 0
	failedCount := 0
	var failedGameNames []string
	completedCount := 0

	for result := range resultQueue {
		completedCount++

		if result.Success {
			fmt.Printf("[%d/%d] %s... ✓ Found (ID: %d)\n", completedCount, totalGames, result.GameName, result.GameID)
			successCount++
		} else {
			// Show error message in verbose mode, otherwise generic "Not found"
			errorMessage := "Not found"
			if *verboseFlag && result.Error != nil {
				errorMessage = result.Error.Error()
			}
			fmt.Printf("[%d/%d] %s... ✗ %s\n", completedCount, totalGames, result.GameName, errorMessage)
			failedCount++
			failedGameNames = append(failedGameNames, result.GameName)
		}
	}

	// Print summary
	fmt.Printf("\n%s\n", strings.Repeat("=", summarySeparator))
	fmt.Println("Summary:")
	fmt.Printf("  Success: %d\n", successCount)
	fmt.Printf("  Failed:  %d\n", failedCount)
	fmt.Printf("  Output:  %s/\n", *outputDirectoryFlag)

	if len(failedGameNames) > 0 {
		fmt.Println("\nFailed games:")
		for _, gameName := range failedGameNames {
			fmt.Printf("  - %s\n", gameName)
		}
	}

	fmt.Printf("\nDone! Check the '%s/' folder for .steam files.\n", *outputDirectoryFlag)
}
