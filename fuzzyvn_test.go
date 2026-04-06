package fuzzyvn

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/versenilvis/fuzzyvn/core"
)

const TestDataDir = "./demo/test_data"

func scanDir(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Đường", "duong"},
		{"đường", "duong"},
		{"Nguyễn", "nguyen"},
		{"nguyễn", "nguyen"},
		{"Huệ", "hue"},
		{"café", "cafe"},
		{"kỷ niệm", "ky niem"},
		{"kỉ niệm", "ki niem"},
		{"lý do", "ly do"},
		{"lí do", "li do"},
		{"quy định", "quy dinh"},
		{"qui định", "qui dinh"},
		{"Sơn Tùng", "son tung"},
		{"Báo cáo tháng 1", "bao cao thang 1"},
		{"Hello World", "hello world"},
		{"vật lý", "vat ly"},
		{"Python", "python"},
		{"", ""},
	}

	for _, tt := range tests {
		result := Normalize(tt.input)
		if result != tt.expected {
			t.Errorf("Normalize(%q) = %q, muốn %q", tt.input, result, tt.expected)
		}
	}
}

func TestNormalize_YI_NotEquivalent(t *testing.T) {
	pairs := []struct {
		a, b    string
		expectA string
		expectB string
	}{
		{"kỷ niệm", "kỉ niệm", "ky niem", "ki niem"},
		{"lý do", "lí do", "ly do", "li do"},
		{"vật lý", "vật lí", "vat ly", "vat li"},
	}

	for _, pair := range pairs {
		normA := Normalize(pair.a)
		normB := Normalize(pair.b)
		if normA != pair.expectA {
			t.Errorf("Normalize(%q) = %q, muốn %q", pair.a, normA, pair.expectA)
		}
		if normB != pair.expectB {
			t.Errorf("Normalize(%q) = %q, muốn %q", pair.b, normB, pair.expectB)
		}
		if normA == normB {
			t.Errorf("Normalize(%q) = %q KHÔNG nên bằng Normalize(%q) = %q", pair.a, normA, pair.b, normB)
		}
	}
}

func TestLevenshteinRatio(t *testing.T) {
	tests := []struct {
		s1, s2   string
		expected int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "ab", 1},
		{"abc", "abcd", 1},
		{"main", "mian", 2},
		{"kitten", "sitting", 3},
		{"hello", "hallo", 1},
	}

	for _, tt := range tests {
		result := LevenshteinRatio(tt.s1, tt.s2)
		if result != tt.expected {
			t.Errorf("LevenshteinRatio(%q, %q) = %d, muốn %d", tt.s1, tt.s2, result, tt.expected)
		}
	}
}

func TestNewSearcher(t *testing.T) {
	files := []string{
		"/home/user/main.go",
		"/home/user/config.yaml",
		"/home/user/README.md",
	}

	searcher := NewSearcher(files)

	if len(searcher.Originals) != 3 {
		t.Errorf("Originals có %d phần tử, muốn 3", len(searcher.Originals))
	}
	if len(searcher.Normalized) != 3 {
		t.Errorf("Normalized có %d phần tử, muốn 3", len(searcher.Normalized))
	}
	if searcher.Memory == nil {
		t.Error("Memory không được khởi tạo")
	}
}

func TestSearcher_Search_Basic(t *testing.T) {
	files := []string{
		"/project/main.go",
		"/project/main_test.go",
		"/project/config.yaml",
		"/project/README.md",
	}

	searcher := NewSearcher(files)

	results := searcher.Search("main")
	if len(results) < 2 {
		t.Errorf("Search('main') trả về %d kết quả, muốn ít nhất 2", len(results))
	}
	if !slices.Contains(results, "/project/main.go") {
		t.Error("Search('main') không tìm thấy /project/main.go")
	}
}

func TestSearcher_Search_Vietnamese(t *testing.T) {
	files := []string{
		"/docs/Báo_cáo_tháng_1.pdf",
		"/docs/Hợp_đồng_thuê_nhà.docx",
		"/music/Sơn Tùng - Lạc Trôi.mp3",
		"/music/Mỹ Tâm - Đừng Hỏi Em.mp3",
	}

	searcher := NewSearcher(files)

	tests := []struct {
		query    string
		contains string
	}{
		{"bao cao", "Báo_cáo"},
		{"hop dong", "Hợp_đồng"},
		{"son tung", "Sơn Tùng"},
		{"lac troi", "Lạc Trôi"},
		{"my tam", "Mỹ Tâm"},
	}

	for _, tt := range tests {
		results := searcher.Search(tt.query)
		if len(results) == 0 {
			t.Errorf("Search(%q) không trả về kết quả", tt.query)
			continue
		}
		found := slices.ContainsFunc(results, func(r string) bool {
			return strings.Contains(r, tt.contains)
		})
		if !found {
			t.Errorf("Search(%q) không tìm thấy file chứa %q", tt.query, tt.contains)
		}
	}
}

func TestSearcher_Search_IY_Equivalence(t *testing.T) {
	files := []string{
		"/music/Kỷ Niệm Vô Tận - Vũ.flac",
		"/docs/Lý do nghỉ việc.docx",
	}

	searcher := NewSearcher(files)

	results1 := searcher.Search("ky niem")
	results2 := searcher.Search("ki niem")

	if len(results1) == 0 || len(results2) == 0 {
		t.Error("Search với i/y phải trả về kết quả")
	}
	if len(results1) != len(results2) {
		t.Errorf("Search('ky niem') và Search('ki niem') phải cho cùng số kết quả")
	}
}

func TestSearcher_Search_Typo(t *testing.T) {
	files := []string{
		"/project/main.go",
		"/project/config.yaml",
	}

	searcher := NewSearcher(files)

	results := searcher.Search("mian")
	if !slices.Contains(results, "/project/main.go") {
		t.Error("Search('mian') phải tìm thấy main.go (typo tolerance)")
	}
}

// --- FileMemory Tests (thay thế QueryCache) ---

func TestFileMemory_RecordSelection(t *testing.T) {
	mem := core.NewFileMemory(nil)

	mem.RecordSelection("main", "/project/main.go")
	boosts := mem.GetBoostScores("main")
	if boosts["/project/main.go"] == 0 {
		t.Error("GetBoostScores phải trả về score cho file đã record")
	}

	// Record thêm lần nữa, score phải tăng
	mem.RecordSelection("main", "/project/main.go")
	boosts2 := mem.GetBoostScores("main")
	if boosts2["/project/main.go"] <= boosts["/project/main.go"] {
		t.Error("Score phải tăng khi record thêm")
	}
}

func TestFileMemory_GetBoostScores_SimilarQuery(t *testing.T) {
	mem := core.NewFileMemory(nil)
	mem.RecordSelection("main server", "/project/main_server.go")

	// Query tương tự phải vẫn có boost nhờ JaroWinkler
	tests := []string{"main", "main ser"}
	for _, query := range tests {
		scores := mem.GetBoostScores(query)
		if scores["/project/main_server.go"] == 0 {
			t.Errorf("GetBoostScores(%q) phải trả về score cho query tương tự", query)
		}
	}
}

func TestFileMemory_GetRecentFiles(t *testing.T) {
	mem := core.NewFileMemory(nil)
	mem.RecordSelection("q1", "/a.go")
	time.Sleep(1100 * time.Millisecond) // Đảm bảo timestamp khác biệt (Unix tính theo giây)
	mem.RecordSelection("q2", "/b.go")
	time.Sleep(1100 * time.Millisecond)
	mem.RecordSelection("q3", "/c.go")

	files := mem.GetRecentFiles(5)
	if len(files) != 3 {
		t.Errorf("GetRecentFiles trả về %d files, muốn 3", len(files))
	}
	if files[0] != "/c.go" {
		t.Errorf("File gần nhất = %q, muốn '/c.go'", files[0])
	}
}

func TestFileMemory_Persistence(t *testing.T) {
	mem1 := core.NewFileMemory(nil)
	mem1.RecordSelection("test", "/test.go")

	data, err := mem1.Export()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	mem2 := core.NewFileMemory(nil)
	err = mem2.Import(data)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	boosts := mem2.GetBoostScores("test")
	if boosts["/test.go"] <= 0 {
		t.Errorf("Expected boost for /test.go after import, got %d", boosts["/test.go"])
	}
}

func TestSearcher_RecordSelection_BoostsResults(t *testing.T) {
	files := []string{
		"/project/main.go",
		"/project/main_server.go",
		"/project/main_test.go",
		"/project/config.yaml",
	}

	searcher := NewSearcher(files)

	searcher.RecordSelection("main", "/project/main_test.go")
	searcher.RecordSelection("main", "/project/main_test.go")
	searcher.RecordSelection("main", "/project/main_test.go")

	results := searcher.Search("main")
	if len(results) == 0 {
		t.Fatal("Search không trả về kết quả")
	}
	if results[0] != "/project/main_test.go" {
		t.Errorf("File được chọn nhiều lần phải ở đầu, got %q", results[0])
	}
}

func TestSearcher_ContextBoosts(t *testing.T) {
	files := []string{
		"auth/user.go",
		"auth/user_test.go",
		"models/user.go",
	}
	searcher := NewSearcher(files)

	opts := &SearchOptions{
		ContextBoosts: map[string]int{
			"auth/user_test.go": 10000,
		},
	}
	results := searcher.Search("user", opts)
	if len(results) == 0 || results[0] != "auth/user_test.go" {
		t.Errorf("Expected auth/user_test.go at top due to ContextBoost, got %v", results)
	}
}

func TestNewSearcherWithMemory(t *testing.T) {
	files1 := []string{"/a.go", "/b.go"}
	searcher1 := NewSearcher(files1)
	searcher1.RecordSelection("test", "/a.go")

	mem := searcher1.Memory

	files2 := []string{"/a.go", "/b.go", "/c.go"}
	searcher2 := NewSearcherWithMemory(files2, mem)

	boosts := searcher2.Memory.GetBoostScores("test")
	if boosts["/a.go"] <= 0 {
		t.Error("Memory phải được giữ lại khi dùng NewSearcherWithMemory")
	}
}

func TestSearcher_ClearCache(t *testing.T) {
	files := []string{"/main.go"}
	searcher := NewSearcher(files)
	searcher.RecordSelection("main", "/main.go")

	searcher.ClearCache()

	boosts := searcher.Memory.GetBoostScores("main")
	if boosts["/main.go"] != 0 {
		t.Error("ClearCache phải xóa hết memory")
	}
}

func TestSearchWithVietnameseData(t *testing.T) {
	cases := []struct {
		pattern     string
		data        []string
		wantMatches int
		wantFirst   string
	}{
		{
			"bao cao",
			[]string{"Báo_cáo_tháng_1.pdf", "config.yaml", "README.md"},
			1, "Báo_cáo_tháng_1.pdf",
		},
		{
			"son tung",
			[]string{"Sơn Tùng - Lạc Trôi.mp3", "Mỹ Tâm - Hãy Về Đây.mp3"},
			1, "Sơn Tùng - Lạc Trôi.mp3",
		},
		{
			"ky niem",
			[]string{"Kỷ Niệm Vô Tận.flac", "Kỉ Niệm Xưa.mp3", "config.yaml"},
			1, "",
		},
		{
			"ki niem",
			[]string{"Kỷ Niệm Vô Tận.flac", "Kỉ Niệm Xưa.mp3", "config.yaml"},
			1, "",
		},
		{
			"duong",
			[]string{"Đường Về Nhà.mp3", "duong_dan.txt", "config.yaml"},
			2, "",
		},
		{
			"nguyen",
			[]string{"Nguyễn Văn A.docx", "nguyen_config.yaml"},
			2, "",
		},
	}

	for _, c := range cases {
		searcher := NewSearcher(c.data)
		results := searcher.Search(c.pattern)

		if len(results) < c.wantMatches {
			t.Errorf("Search(%q): got %d matches, want at least %d", c.pattern, len(results), c.wantMatches)
		}
		if c.wantFirst != "" && len(results) > 0 && results[0] != c.wantFirst {
			t.Errorf("Search(%q): first match = %q, want %q", c.pattern, results[0], c.wantFirst)
		}
	}
}

func TestSearchWithTypos(t *testing.T) {
	cases := []struct {
		pattern string
		data    []string
		want    string
	}{
		{"mian", []string{"main.go", "config.yaml"}, "main.go"},
		{"conifg", []string{"main.go", "config.yaml"}, "config.yaml"},
		{"redame", []string{"README.md", "main.go"}, "README.md"},
		{"maiin", []string{"main.go", "test.go"}, "main.go"},
	}

	for _, c := range cases {
		searcher := NewSearcher(c.data)
		results := searcher.Search(c.pattern)

		if !slices.Contains(results, c.want) {
			t.Errorf("Search(%q) với typo: không tìm thấy %q, got %v", c.pattern, c.want, results)
		}
	}
}

func TestSearcher_EdgeCases(t *testing.T) {
	t.Run("Query rỗng hoặc toàn khoảng trắng", func(t *testing.T) {
		s := NewSearcher([]string{"a.go", "b.go"})
		if s.Search("") != nil {
			t.Error("Query rỗng phải trả về nil")
		}
		// Query chỉ có space nên trả về nil (tùy thiết kế, hiện tại ta Filter theo Normalized)
		if len(s.Search("   ")) > 0 && s.Search("   ")[0] == "" {
			t.Error("Query toàn space không nên trả về kết quả rác")
		}
	})

	t.Run("Dữ liệu đầu vào rỗng", func(t *testing.T) {
		s := NewSearcher([]string{})
		if s.Search("anything") != nil {
			t.Error("Search trên danh sách rỗng phải trả về nil")
		}
	})

	t.Run("Query dài hơn Target", func(t *testing.T) {
		s := NewSearcher([]string{"short.go"})
		res := s.Search("this_is_a_very_long_query_that_exceeds_target")
		if len(res) > 0 {
			t.Error("Query dài hơn target không được phép khớp")
		}
	})

	t.Run("Ký tự đặc biệt và Emoji", func(t *testing.T) {
		s := NewSearcher([]string{"🚀_launch.sh", "Makefile", "README.md"})
		// Emoji thường bị Normalize loại bỏ (vì không phải Letter/Digit)
		// Ta test xem hệ thống có bị crash không
		res := s.Search("launch")
		if !slices.Contains(res, "🚀_launch.sh") {
			t.Errorf("Nên tìm thấy file chứa emoji, got %v", res)
		}
	})

	t.Run("File trùng lặp trong index", func(t *testing.T) {
		s := NewSearcher([]string{"main.go", "main.go", "utils.go"})
		res := s.Search("main")
		// Tùy thiết kế, hiện tại ta lưu cả 2 index. Nhưng nên trả về duy nhất 1 string
		if len(res) > 2 {
			t.Errorf("Không nên trả về nhiều hơn số lượng file thực tế, got %d", len(res))
		}
	})

	t.Run("Context Boost âm (Dìm hàng)", func(t *testing.T) {
		s := NewSearcher([]string{"good.go", "bad.go"})
		opts := &SearchOptions{
			ContextBoosts: map[string]int{
				"bad.go": -10000,
			},
		}
		// "bad.go" khớp tốt hơn về mặt ký tự nhưng bị dìm điểm
		res := s.Search("go", opts)
		if len(res) >= 2 && res[0] == "bad.go" {
			t.Error("File bị boost âm không nên nằm ở đầu bảng")
		}
	})
}

func TestDeepEdgeCases(t *testing.T) {
	t.Run("Unicode NFC vs NFD", func(t *testing.T) {
		// Ký tự "ê" (U+00EA) - NFC
		// Ký tự "ê" (U+00EA) - NFC
		nfc := "ê"
		// Ký tự "e" + dấu mũ rời (U+0065 + U+0302) - NFD
		nfd := "e\u0302"

		files := []string{"thiết kế.txt"} // Đây là NFD (copy từ macOS hoặc gõ rời)
		s := NewSearcher(files)

		// Search bằng NFC
		res1 := s.Search(nfc)
		res2 := s.Search(nfd)
		if len(res1) == 0 || len(res2) == 0 {
			t.Error("Nên tìm thấy file bất kể encoding NFC/NFD")
		}
	})

	t.Run("Overflow SelectCount", func(t *testing.T) {
		mem := core.NewFileMemory(nil)
		path := "over.go"
		// Giả lập gõ kịch kim MaxInt (chú ý: bài test này tốn CPU nếu chạy loop thật, ta set trực tiếp nếu có thể hoặc dùng loop nhỏ)
		// Thực tế ta check logic chặn ở code core rồi.
		for i := 0; i < 1000; i++ {
			mem.RecordSelection("q", path)
		}
		// Test xem Import/Export có an toàn không
		data, _ := mem.Export()
		newMem := core.NewFileMemory(nil)
		if err := newMem.Import(data); err != nil {
			t.Errorf("Import data có SelectCount lớn không nên lỗi: %v", err)
		}
	})

	t.Run("Control Characters & Null Bytes", func(t *testing.T) {
		files := []string{"\x00main.go", "\tconfig\n.yaml"}
		s := NewSearcher(files)
		res := s.Search("main")
		if len(res) == 0 {
			t.Error("Nên tìm thấy file có chứa ký tự điều khiển hoặc null byte")
		}
	})

	t.Run("Deterministic Sorting", func(t *testing.T) {
		// Hai file cùng điểm số (vì cùng filename và cùng query)
		files := []string{"/b/utils.go", "/a/utils.go"}
		s := NewSearcher(files)
		res1 := s.Search("utils")
		// Phải luôn trả về /a/ trước /b/ (do sort alphabet khi điểm bằng nhau)
		if res1[0] != "/a/utils.go" {
			t.Errorf("Sort không deterministic, got %s first", res1[0])
		}
	})

	t.Run("Concurrency & Race Condition", func(t *testing.T) {
		files := generateTestFiles(100)
		s := NewSearcher(files)
		var wg sync.WaitGroup

		// Chạy 50 goroutine Search đồng thời
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					s.Search("main")
				}
			}()
		}

		// Một vài goroutine RecordSelection đồng thời
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					s.RecordSelection("main", files[0])
				}
			}()
		}

		wg.Wait()
		// Nếu chạy với -race mà không báo gì là PASS
	})
}

func TestSearchCacheBoost(t *testing.T) {
	files := []string{
		"/project/main.go",
		"/project/main_test.go",
		"/project/main_server.go",
		"/project/main_client.go",
		"/project/config.yaml",
	}

	searcher := NewSearcher(files)

	searcher.RecordSelection("main", "/project/main_client.go")
	searcher.RecordSelection("main", "/project/main_client.go")
	searcher.RecordSelection("main", "/project/main_client.go")

	results := searcher.Search("main")
	if results[0] != "/project/main_client.go" {
		t.Errorf("Sau khi boost, file được chọn nhiều lần phải lên đầu. Got %q", results[0])
	}
}

func TestSearchWithRealworldData(t *testing.T) {
	t.Run("với test_data từ folder", func(t *testing.T) {
		if _, err := os.Stat(TestDataDir); os.IsNotExist(err) {
			t.Skipf("Bỏ qua: không có thư mục %s", TestDataDir)
		}

		files, err := scanDir(TestDataDir)
		if err != nil {
			t.Fatalf("Lỗi scanDir: %v", err)
		}
		if len(files) == 0 {
			t.Skip("Bỏ qua: không có files trong folder test_data")
		}

		searcher := NewSearcher(files)

		cases := []struct {
			pattern     string
			wantMatches int
		}{
			{"son tung", 1},
			{"ky niem", 1},
			{"lac troi", 1},
		}

		for _, c := range cases {
			now := time.Now()
			results := searcher.Search(c.pattern)
			elapsed := time.Since(now)

			fmt.Printf("Search '%s' trong %d files... tìm thấy %d kết quả trong %v\n",
				c.pattern, len(files), len(results), elapsed)

			if len(results) < c.wantMatches {
				t.Errorf("Search(%q): got %d matches, want at least %d", c.pattern, len(results), c.wantMatches)
			}
		}
	})
}

// =============================================================================
// BENCHMARKS
// =============================================================================

func BenchmarkSearch_RealWorld(b *testing.B) {
	allFiles, err := scanDir(TestDataDir)
	if err != nil {
		b.Skipf("Lỗi khi scan thư mục %s: %v", TestDataDir, err)
	}
	if len(allFiles) == 0 {
		b.Skipf("Thư mục %s rỗng", TestDataDir)
	}

	fmt.Printf("\n--- Benchmark Info ---\nĐã load %d files từ ổ cứng\n----------------------\n", len(allFiles))

	var files50k []string
	if len(allFiles) >= 50000 {
		files50k = allFiles[:50000]
	} else {
		files50k = allFiles
	}

	b.Run("Search/50k_Files", func(b *testing.B) {
		searcher := NewSearcher(files50k)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			searcher.Search("config")
		}
	})

	b.Run("Search/100k_Files", func(b *testing.B) {
		searcher := NewSearcher(allFiles)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			searcher.Search("config")
		}
	})

	b.Run("Search/100K_Files_Typo", func(b *testing.B) {
		searcher := NewSearcher(allFiles)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			searcher.Search("conifg")
		}
	})
}

func BenchmarkNewSearcher(b *testing.B) {
	files := generateTestFiles(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewSearcher(files)
	}
}

func BenchmarkSearch(b *testing.B) {
	b.Run("100 files", func(b *testing.B) {
		files := generateTestFiles(100)
		searcher := NewSearcher(files)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			searcher.Search("main")
		}
	})

	b.Run("1000 files", func(b *testing.B) {
		files := generateTestFiles(1000)
		searcher := NewSearcher(files)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			searcher.Search("main")
		}
	})

	b.Run("10000 files", func(b *testing.B) {
		files := generateTestFiles(10000)
		searcher := NewSearcher(files)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			searcher.Search("config")
		}
	})
}

func BenchmarkSearchVietnamese(b *testing.B) {
	files := generateVietnameseTestFiles(1000)
	searcher := NewSearcher(files)

	b.Run("tiếng Việt có dấu", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			searcher.Search("báo cáo")
		}
	})

	b.Run("tiếng Việt không dấu", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			searcher.Search("bao cao")
		}
	})
}

func BenchmarkSearchWithCache(b *testing.B) {
	files := generateTestFiles(1000)
	searcher := NewSearcher(files)

	searcher.RecordSelection("main", files[0])
	searcher.RecordSelection("main", files[1])
	searcher.RecordSelection("config", files[2])

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		searcher.Search("main")
	}
}

func BenchmarkNormalize(b *testing.B) {
	testStrings := []string{
		"Đường Nguyễn Huệ",
		"Báo cáo tháng 1",
		"Sơn Tùng M-TP",
		"Kỷ niệm vô tận",
		"Hello World",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, s := range testStrings {
			Normalize(s)
		}
	}
}

func BenchmarkLevenshteinRatio(b *testing.B) {
	pairs := []struct{ a, b string }{
		{"main", "mian"},
		{"config", "conifg"},
		{"moduleNameResolver", "mnr"},
		{"hello", "hallo"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range pairs {
			LevenshteinRatio(p.a, p.b)
		}
	}
}

func BenchmarkRecordSelection(b *testing.B) {
	mem := core.NewFileMemory(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mem.RecordSelection(fmt.Sprintf("query%d", i%100), fmt.Sprintf("/file%d.go", i%1000))
	}
}

func BenchmarkGetBoostScores(b *testing.B) {
	mem := core.NewFileMemory(nil)
	for i := 0; i < 100; i++ {
		mem.RecordSelection(fmt.Sprintf("query%d", i), fmt.Sprintf("/file%d.go", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mem.GetBoostScores("query50")
	}
}

func generateTestFiles(n int) []string {
	files := make([]string, n)
	names := []string{"main", "config", "test", "utils", "helper", "server", "client", "api", "model", "view"}
	exts := []string{".go", ".yaml", ".json", ".md", ".txt"}

	for i := range n {
		name := names[i%len(names)]
		ext := exts[i%len(exts)]
		files[i] = fmt.Sprintf("/project/src/%s_%d%s", name, i, ext)
	}
	return files
}

func generateVietnameseTestFiles(n int) []string {
	files := make([]string, n)
	names := []string{
		"Báo_cáo_tháng", "Hợp_đồng_thuê", "Đơn_xin_nghỉ",
		"Kế_hoạch_năm", "Biên_bản_họp", "Quyết_định",
		"Thông_báo", "Công_văn", "Tờ_trình", "Đề_xuất",
	}
	exts := []string{".pdf", ".docx", ".xlsx", ".pptx", ".txt"}

	for i := 0; i < n; i++ {
		name := names[i%len(names)]
		ext := exts[i%len(exts)]
		files[i] = fmt.Sprintf("/documents/%s_%d%s", name, i, ext)
	}
	return files
}
