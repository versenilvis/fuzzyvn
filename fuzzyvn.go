package fuzzyvn

import (
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/versenilvis/fuzzyvn/core"
)

// SearchOptions: Tùy chọn nâng cao khi tìm kiếm
type SearchOptions struct {
	ContextBoosts map[string]int // Cho phép app truyền điểm cộng từ ngoài vào (ví dụ: sibling files)
}

// MatchResult: Kết quả tìm kiếm thô sau khi chấm điểm
type MatchResult struct {
	Str   string
	Score int
}

// Searcher: Object chính để thực hiện tìm kiếm
type Searcher struct {
	Originals     []string            // Danh sách file gốc để trả về kết quả
	Normalized    [][]byte            // Danh sách file đã chuẩn hóa (byte) để search nhanh
	FilenamesOnly []string            // Chỉ tên file đã chuẩn hóa (dùng cho typo matching)
	FilePathToIdx map[string]int      // Map nhanh từ path sang index
	Memory        *core.FileMemory    // Hệ thống ghi nhớ hành vi (Frecency)
	Filter        *core.UnigramFilter // Bộ lọc Bitset (K-of-N)
	baseStarts    []int               // Vị trí bắt đầu của filename trong từng path
	scorePool     *sync.Pool          // Pool để tái sử dụng []int buffer (tránh race condition)
}

/*
NewSearcher: Khởi tạo Searcher và xây dựng Index (Unigram Bitset + Path Normalization)
- items: Danh sách các đường dẫn file (ví dụ: lấy từ git ls-files hoặc os.ReadDir)
*/
func NewSearcher(items []string) *Searcher {
	numItems := len(items)
	originals := make([]string, numItems)
	normPaths := make([][]byte, numItems)
	normNames := make([]string, numItems)
	pathMap := make(map[string]int, numItems)
	baseStarts := make([]int, numItems)

	for i, item := range items {
		originals[i] = item

		// Tách filename từ path gốc
		bStart := 0
		for j := len(item) - 1; j >= 0; j-- {
			if item[j] == '/' || item[j] == '\\' {
				bStart = j + 1
				break
			}
		}

		filename := item[bStart:]
		// Priority String format: "filename fullpath"
		// Target sẽ là "main.go src/main.go" (sau normalize)
		// baseStart trỏ vào vị trí dấu space giữa filename và path
		normFilename := core.Normalize(filename)
		baseStarts[i] = len(normFilename)

		priorityString := filename + " " + item
		normPaths[i] = []byte(core.Normalize(priorityString))
		normNames[i] = normFilename
		pathMap[item] = i
	}

	return &Searcher{
		Originals:     originals,
		Normalized:    normPaths,
		FilenamesOnly: normNames,
		FilePathToIdx: pathMap,
		Memory:        core.NewFileMemory(nil),
		Filter:        core.NewUnigramFilter(normPaths),
		baseStarts:    baseStarts,
		scorePool: &sync.Pool{
			New: func() interface{} {
				buf := make([]int, numItems)
				for i := range buf {
					buf[i] = math.MinInt
				}
				return &buf
			},
		},
	}
}

/*
NewSearcherWithMemory: Khởi tạo Searcher với cache có sẵn
*/
func NewSearcherWithMemory(items []string, memory *core.FileMemory) *Searcher {
	s := NewSearcher(items)
	if memory != nil {
		s.Memory = memory
	}
	return s
}

/*
Search: Thực hiện tìm kiếm fuzzy trên tập dữ liệu đã index
- opts: Tùy chọn (ContextBoosts...)
*/
func (s *Searcher) Search(query string, opts ...*SearchOptions) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	// Nếu query có chữ viết hoa -> giữ nguyên case để chấm điểm chính xác hơn
	queryNorm := core.Normalize(query)
	queryPattern := []byte(queryNorm)

	memoryBoosts := s.Memory.GetBoostScores(query)

	var matches []core.FuzzyMatch
	// lọc bớt các file chắc chắn không khớp
	candidates := s.Filter.Filter(queryPattern)
	
	if candidates != nil {
		// chỉ chấm điểm các candidates
		matches = core.FuzzyFindFiltered(queryPattern, s.Normalized, candidates, s.baseStarts)
	} else {
		// fallback: full scan parallel (query quá ngắn hoặc filter không hỗ trợ)
		matches = core.FuzzyFindParallel(queryPattern, s.Normalized, s.baseStarts)
	}

	// nếu không tìm thấy gì bằng Fuzzy -> Levenshtein
	if len(matches) == 0 && len(queryNorm) >= 3 {
		matches = s.findButTypo(queryNorm)
	}

	if len(matches) == 0 {
		return nil
	}

	// xếp hạng và áp dụng boosts
	scoreBufPtr := s.scorePool.Get().(*[]int)
	scoreBuf := *scoreBufPtr
	defer func() {
		// reset buffer trước khi trả lại pool
		for i := range scoreBuf {
			scoreBuf[i] = math.MinInt
		}
		s.scorePool.Put(scoreBufPtr)
	}()

	for _, m := range matches {
		scoreBuf[m.Index] = m.Score
	}

	rankedResults := make([]MatchResult, 0, len(matches))
	for _, m := range matches {
		filePath := s.Originals[m.Index]
		finalScore := m.Score

		// Boost 1: Từ Memory (Frecency)
		if boost, exists := memoryBoosts[filePath]; exists {
			finalScore += boost
		}

		// Boost 2: Từ Context (Sibling files...)
		if len(opts) > 0 && opts[0] != nil && opts[0].ContextBoosts != nil {
			if boost, exists := opts[0].ContextBoosts[filePath]; exists {
				finalScore += boost
			}
		}

		rankedResults = append(rankedResults, MatchResult{
			Str:   filePath,
			Score: finalScore,
		})
	}

	// Sort để đảm bảo thứ tự deterministic (alphabet khi cùng điểm)
	sort.Slice(rankedResults, func(i, j int) bool {
		if rankedResults[i].Score == rankedResults[j].Score {
			return rankedResults[i].Str < rankedResults[j].Str
		}
		return rankedResults[i].Score > rankedResults[j].Score
	})

	// Trả về Top 20 (hoặc tùy cấu hình)
	limit := 20
	if len(rankedResults) < limit {
		limit = len(rankedResults)
	}
	
	finalStrings := make([]string, limit)
	for i, res := range rankedResults[:limit] {
		finalStrings[i] = res.Str
	}

	return finalStrings
}

/*
findButTypo: Tìm kiếm gợi ý khi người dùng gõ sai chính tả (dựa trên Levenshtein)
*/
func (s *Searcher) findButTypo(query string) []core.FuzzyMatch {
	numItems := len(s.FilenamesOnly)
	if numItems == 0 {
		return nil
	}

	threshold := len(query) / 4
	if threshold < 1 {
		threshold = 1
	}

	numCPUs := runtime.GOMAXPROCS(0)
	chunkSize := (numItems + numCPUs - 1) / numCPUs

	var wg sync.WaitGroup
	resultChan := make(chan []core.FuzzyMatch, numCPUs)

	for i := range numCPUs {
		start := i * chunkSize
		if start >= numItems {
			break
		}
		end := start + chunkSize
		if end > numItems {
			end = numItems
		}

		wg.Add(1)
		go func(s0, e int) {
			defer wg.Done()
			var local []core.FuzzyMatch
			for j := s0; j < e; j++ {
				dist := core.LevenshteinRatio(query, s.FilenamesOnly[j])
				if dist <= threshold {
					local = append(local, core.FuzzyMatch{
						Index: j,
						Score: 100 - dist*10,
					})
				}
			}
			resultChan <- local
		}(start, end)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var matches []core.FuzzyMatch
	for chunk := range resultChan {
		matches = append(matches, chunk...)
	}
	return matches
}

/*
RecordSelection: Ghi lại việc chọn file để tăng điểm Frecency
*/
func (s *Searcher) RecordSelection(query, filePath string) {
	s.Memory.RecordSelection(query, filePath)
}

/*
ClearCache: Xóa sạch bộ nhớ lịch sử
*/
func (s *Searcher) ClearCache() {
	s.Memory = core.NewFileMemory(nil)
}

// Alias cho các hàm helper (để thuận tiện cho người dùng package)
func Normalize(s string) string {
	return core.Normalize(s)
}

func LevenshteinRatio(s1, s2 string) int {
	return core.LevenshteinRatio(s1, s2)
}
