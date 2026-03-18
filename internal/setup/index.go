package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marijus001/tokara/internal/api"
	"github.com/marijus001/tokara/internal/prompt"
)

// Excluded directories and files from indexing.
var excludedDirs = map[string]bool{
	"node_modules": true, "dist": true, "build": true, ".git": true,
	".next": true, "__pycache__": true, ".venv": true, "vendor": true,
	"target": true, ".claude": true, ".agents": true, ".superpowers": true,
}

var excludedFiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"go.sum": true, ".DS_Store": true, "Thumbs.db": true,
}

var supportedExts = map[string]bool{
	".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".py": true, ".go": true, ".rs": true, ".rb": true,
	".java": true, ".c": true, ".cpp": true, ".h": true,
	".cs": true, ".php": true, ".swift": true, ".kt": true,
	".css": true, ".html": true, ".sql": true, ".sh": true,
	".yaml": true, ".yml": true, ".toml": true, ".json": true,
	".md": true, ".txt": true,
}

// RunIndex walks a directory, collects source files, and sends them to the API.
func RunIndex(client *api.Client, dirPath string, projectID string) error {
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", absPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", absPath)
	}

	prompt.Banner()
	fmt.Printf("  Indexing %s\n", absPath)
	prompt.Blank()

	// Walk and collect files
	var files []api.IngestFile
	var totalSize int64
	var skipped int

	err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip excluded directories
		if info.IsDir() {
			if excludedDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip excluded files
		if excludedFiles[info.Name()] {
			skipped++
			return nil
		}

		// Skip unsupported extensions
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if !supportedExts[ext] {
			skipped++
			return nil
		}

		// Skip large files (>1MB)
		if info.Size() > 1_000_000 {
			skipped++
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(absPath, path)
		files = append(files, api.IngestFile{
			Path:    relPath,
			Content: string(content),
		})
		totalSize += info.Size()

		return nil
	})

	if err != nil {
		return fmt.Errorf("walk error: %w", err)
	}

	if len(files) == 0 {
		prompt.Fail("No source files found")
		return nil
	}

	fmt.Printf("  Found %d files (%s), skipped %d\n", len(files), formatBytes(totalSize), skipped)
	prompt.Blank()

	if !prompt.Confirm("Send to Tokara API for indexing?", true) {
		prompt.Info("Cancelled")
		return nil
	}

	prompt.Blank()
	fmt.Printf("  Uploading...")

	// Send in batches of 50 files
	batchSize := 50
	totalChunks := 0
	for i := 0; i < len(files); i += batchSize {
		end := i + batchSize
		if end > len(files) {
			end = len(files)
		}
		batch := files[i:end]

		resp, err := client.Ingest(api.IngestRequest{
			Files:     batch,
			Clear:     i == 0, // Clear on first batch only
			ProjectID: projectID,
		})
		if err != nil {
			fmt.Println()
			return fmt.Errorf("ingest error: %w", err)
		}
		totalChunks += resp.Chunks
		fmt.Printf(".")
	}
	fmt.Println()
	prompt.Blank()

	prompt.OK(fmt.Sprintf("Indexed %d files → %d chunks", len(files), totalChunks))
	prompt.Info("Your codebase is now searchable via the Tokara API")
	prompt.Blank()

	return nil
}

func formatBytes(b int64) string {
	if b >= 1_000_000 {
		return fmt.Sprintf("%.1f MB", float64(b)/1_000_000)
	}
	if b >= 1_000 {
		return fmt.Sprintf("%.1f KB", float64(b)/1_000)
	}
	return fmt.Sprintf("%d B", b)
}
