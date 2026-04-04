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
)

//go:embed index.html
var challengeHTML string

var (
	searcher     *fuzzyvn.Searcher
	searcherLock sync.RWMutex
	globalCache  *fuzzyvn.QueryCache
)

type SearchResult struct {
	Path    string `json:"path"`
	Score   int    `json:"score"`
	Boosted bool   `json:"boosted"`
}

type SearchResponse struct {
	CachedFiles []string       `json:"cached_files"`
	Results     []SearchResult `json:"results"`
}

type SelectionRequest struct {
	Query string `json:"query"`
	Path  string `json:"path"`
}

type CacheInfo struct {
	Size          int      `json:"size"`
	RecentQueries []string `json:"recent_queries"`
	RecentFiles   []string `json:"recent_files"`
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
	if globalCache != nil {
		newSearcher = fuzzyvn.NewSearcherWithCache(tempFiles, globalCache)
	} else {
		newSearcher = fuzzyvn.NewSearcher(tempFiles)
		globalCache = newSearcher.GetCache()
	}

	searcherLock.Lock()
	searcher = newSearcher
	searcherLock.Unlock()

	fmt.Printf("Indexed %d files. Cache: %d queries\n", len(tempFiles), globalCache.Size())
}

func search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		json.NewEncoder(w).Encode(SearchResponse{
			CachedFiles: []string{},
			Results:     []SearchResult{},
		})
		return
	}

	searcherLock.RLock()
	defer searcherLock.RUnlock()

	if searcher == nil {
		json.NewEncoder(w).Encode(SearchResponse{
			CachedFiles: []string{},
			Results:     []SearchResult{},
		})
		return
	}

	var cachedFiles []string
	if searcher.Cache != nil {
		cachedFiles = searcher.Cache.GetCachedFiles(query, 5)
	}

	var boostedFiles map[string]int
	if searcher.Cache != nil {
		boostedFiles = searcher.Cache.GetBoostScores(query)
	}

	matchedStrings := searcher.Search(query)

	cachedSet := make(map[string]bool)
	for _, f := range cachedFiles {
		cachedSet[f] = true
	}

	results := []SearchResult{}
	maxRes := 20

	for _, str := range matchedStrings {
		if cachedSet[str] {
			continue
		}

		_, isBoosted := boostedFiles[str]
		results = append(results, SearchResult{
			Path:    str,
			Score:   0,
			Boosted: isBoosted,
		})

		if len(results) >= maxRes {
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{
		CachedFiles: cachedFiles,
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

func cacheInfo(w http.ResponseWriter, r *http.Request) {
	searcherLock.RLock()
	defer searcherLock.RUnlock()

	info := CacheInfo{
		Size:          0,
		RecentQueries: []string{},
		RecentFiles:   []string{},
	}

	if globalCache != nil {
		info.Size = globalCache.Size()
		info.RecentQueries = globalCache.GetRecentQueries(10)
		info.RecentFiles = globalCache.GetAllRecentFiles(5)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func main() {
	go indexFiles("./test_data")

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/search", search)
	http.HandleFunc("/record-selection", recordSelection)
	http.HandleFunc("/cache-info", cacheInfo)

	fmt.Println("Server running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
