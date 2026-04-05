package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/versenilvis/fuzzyvn"
	"github.com/versenilvis/fuzzyvn/core"
)

//go:embed index.html
var challengeHTML string

var (
	searcher     *fuzzyvn.Searcher
	searcherLock sync.RWMutex
	globalMemory *core.FileMemory
)

type SearchResult struct {
	Path    string `json:"path"`
	Score   int    `json:"score"`
	Boosted bool   `json:"boosted"`
}

type SearchResponse struct {
	RecentFiles []string       `json:"recent_files"`
	Results     []SearchResult `json:"results"`
}

type SelectionRequest struct {
	Query string `json:"query"`
	Path  string `json:"path"`
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	tmpl := challengeHTML
	t, _ := template.New("index").Parse(tmpl)
	t.Execute(w, nil)
}

func indexFiles(rootPath string) {
	fmt.Println("Scanning files from directory:", rootPath)
	tempFiles := []string{}

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			tempFiles = append(tempFiles, path)
		}
		return nil
	})
	if err != nil {
		log.Println("Error walking directory:", err)
		return
	}

	var newSearcher *fuzzyvn.Searcher
	if globalMemory != nil {
		newSearcher = fuzzyvn.NewSearcherWithMemory(tempFiles, globalMemory)
	} else {
		newSearcher = fuzzyvn.NewSearcher(tempFiles)
		globalMemory = newSearcher.Memory
	}

	searcherLock.Lock()
	searcher = newSearcher
	searcherLock.Unlock()

	fmt.Printf("Indexed %d files\n", len(tempFiles))
}

func search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		json.NewEncoder(w).Encode(SearchResponse{
			RecentFiles: []string{},
			Results:     []SearchResult{},
		})
		return
	}

	searcherLock.RLock()
	defer searcherLock.RUnlock()

	if searcher == nil {
		json.NewEncoder(w).Encode(SearchResponse{
			RecentFiles: []string{},
			Results:     []SearchResult{},
		})
		return
	}

	recentFiles := globalMemory.GetRecentFiles(5)
	boostedFiles := globalMemory.GetBoostScores(query)
	matchedStrings := searcher.Search(query)

	results := []SearchResult{}
	for _, str := range matchedStrings {
		_, isBoosted := boostedFiles[str]
		results = append(results, SearchResult{
			Path:    str,
			Score:   0,
			Boosted: isBoosted,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{
		RecentFiles: recentFiles,
		Results:     results,
	})
}

func recordSelection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	searcherLock.RLock()
	defer searcherLock.RUnlock()

	if searcher != nil {
		searcher.RecordSelection(req.Query, req.Path)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	go indexFiles("./test_data")

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/search", search)
	http.HandleFunc("/record-selection", recordSelection)

	fmt.Println("Server running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
