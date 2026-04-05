//go:build ignore

package main

import (
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/versenilvis/fuzzyvn"
)

func main() {
	rootPath := "./demo/test_data"
	fmt.Println("CLI DEBUGGER: Scanning...", rootPath)

	var files []string
	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Indexed %d files\n\n", len(files))

	searcher := fuzzyvn.NewSearcher(files)

	queries := []string{"main", "mian", "main.go", "mian.go"}

	for _, query := range queries {
		fmt.Printf("Query: '%s'\n", query)

		for _, f := range files {
			if strings.Contains(f, query) {
				fmt.Println("   [FOUND ON DISK]:", f)
			}
		}

		results := searcher.Search(query)
		fmt.Printf("%d matches, showing top 5:\n", len(results))
		limit := 5
		if len(results) < limit {
			limit = len(results)
		}
		for i := 0; i < limit; i++ {
			fmt.Printf("   #%d: %s\n", i+1, results[i])
		}
		fmt.Println()
	}
}