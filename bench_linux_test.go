//go:build linux

package fuzzyvn

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"testing"
)

// scanLinuxFiles quét filesystem Linux để lấy danh sách file thật
// ưu tiên /usr vì chắc chắn có > 100k files trên hầu hết distro
func scanLinuxFiles(limit int) []string {
	roots := []string{"/usr", "/etc", "/var", "/opt"}
	var files []string

	for _, root := range roots {
		if len(files) >= limit {
			break
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return filepath.SkipDir
			}
			if len(files) >= limit {
				return filepath.SkipAll
			}
			if !d.IsDir() {
				files = append(files, path)
			}
			return nil
		})
	}

	return files
}

func BenchmarkLinux100k(b *testing.B) {
	limit := 100_000
	files := scanLinuxFiles(limit)
	
	fmt.Printf("\n--- Linux Benchmark Check ---\n")
	fmt.Printf("Total files scanned: %d\n", len(files))
	if len(files) < limit {
		b.Fatalf("ERROR: Hệ thống chỉ có %d files, không đủ %d để benchmark", len(files), limit)
	}
	
	fmt.Println("Sample (first 10 files):")
	for i := 0; i < 10; i++ {
		fmt.Printf("  [%d] %s\n", i+1, files[i])
	}
	fmt.Printf("-----------------------------\n")

	searcher := NewSearcher(files)

	queries := []struct {
		name  string
		query string
	}{
		{"ascii_short", "main"},
		{"ascii_ext", "config.yaml"},
		{"path_like", "lib/python"},
		{"typo", "mian"},
		{"long_query", "application.properties"},
	}

	for _, q := range queries {
		b.Run(q.name, func(b *testing.B) {
			b.ResetTimer()
			for b.Loop() {
				searcher.Search(q.query)
			}
		})
	}
}

func BenchmarkLinux100k_NewSearcher(b *testing.B) {
	files := scanLinuxFiles(100_000)
	if len(files) < 50_000 {
		b.Skipf("cần ít nhất 50k files, chỉ tìm thấy %d", len(files))
	}

	b.ResetTimer()
	for b.Loop() {
		NewSearcher(files)
	}
}
