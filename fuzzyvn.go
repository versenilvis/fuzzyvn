/*
----------------
Author: verse91
License: 0BSD
----------------

fuzzyvn.go Structure:
├── Types + Pool
│   ├── CacheEntry
│   ├── QueryCache
│   ├── Searcher
│   ├── MatchResult
│   ├── FuzzyMatch
│   └── Scoring constants
├── Utility Functions
│   ├── abs
│	├── isSeparator
│   ├── countWordMatches
│	├── fastSubstring
│   ├── Normalize
│   ├── LevenshteinRatio
│   └── isWordBoundary
├── Fuzzy Matcher - zero-dependency, greedy algorithm
│   ├── fuzzyScoreGreedy
│   ├── FuzzyFind
│   └── FuzzyFindParallel
├── QueryCache Methods
│   ├── querySimilarity (private)
│   ├── moveToFront (private)
│   ├── evictIfNeeded (private)
│   ├── NewQueryCache
│   ├── SetMaxQueries
│   ├── SetBoostScore
│   ├── RecordSelection
│   ├── GetBoostScores
│   ├── GetRecentQueries
│   ├── GetCachedFiles
│   ├── GetAllRecentFiles
│   ├── Size
│   └── Clear
└── Searcher Methods

	├── NewSearcher
	├── NewSearcherWithCache
	├── Search
	├── RecordSelection
	├── GetCache
	└── ClearCache
*/
package fuzzyvn

import (
	"math"
	"path/filepath"
	"slices"
	"strings"

	"github.com/versenilvis/fuzzyvn/core"
)

// =============================================================================

// Struct
// =============================================================================

/*
  - Boost score tính theo SelectCount: File chọn nhiều lần → điểm boost cao hơn:
    boost = boostScore * similarity * SelectCount / 100
  - Khi vượt giới hạn (maxPerQuery = 5): Entry có SelectCount thấp nhất bị xóa
  - Lần search sau: Files có SelectCount cao được ưu tiên lên đầu
*/
type Searcher struct {
	Originals     []string       // Data gốc (có dấu, viết hoa thường lộn xộn bla bla). Dùng để trả về kết quả hiển thị
	Normalized    [][]byte       // Data đã chuẩn hóa cho fuzzy search, lưu dưới dạng byte để tìm kiếm nhanh
	FilenamesOnly []string       // Chỉ chứa tên file đã chuẩn hóa (bỏ đường dẫn). Dùng cho thuật toán Levenshtein (sửa lỗi chính tả)
	FilePathToIdx map[string]int // Nhằm mục đích không phải tạo lại mỗi lần Search
	Cache         *QueryCache    // Để lấy dữ liệu lịch sử
	scoreBuf      []int          // Pre-alloc flat array cho Search(), reuse qua các lần gọi
}

/*
- Struct tạm thời dùng để gom kết quả và điểm số lại để sắp xếp trước khi trả về cho người dùng
*/
type MatchResult struct {
	Str   string
	Score int
}

// =============================================================================
// Searcher
// =============================================================================

/*
- NewSearcher: Tạo Searcher mới
- items: Danh sách đường dẫn file cần index
*/
func NewSearcher(items []string) *Searcher {
	originals := make([]string, len(items))
	normPaths := make([][]byte, len(items))
	normNames := make([]string, len(items))
	pathMap := make(map[string]int, len(items))

	for i, item := range items {
		originals[i] = item
		filename := filepath.Base(item)
		// Ưu tiên tên file, theo path thì điểm thấp hơn
		priorityString := filename + " " + item
		normPaths[i] = []byte(core.Normalize(priorityString))
		normNames[i] = core.Normalize(filename)

		// Map trong cache để sau này server tìm trong các file gốc nhanh hơn
		pathMap[item] = i
	}

	// Pre-alloc score buffer cho Search(), fill sentinel
	scoreBuf := make([]int, len(items))
	for i := range scoreBuf {
		scoreBuf[i] = math.MinInt
	}

	return &Searcher{
		Originals:     items,
		Normalized:    normPaths,
		FilenamesOnly: normNames,
		FilePathToIdx: pathMap,
		Cache:         core.NewQueryCache(),
		scoreBuf:      scoreBuf,
	}
}

/*
- NewSearcherWithCache: Tạo Searcher mới với cache có sẵn
- items: Danh sáng đường dẫn file cần index
- cache: QueryCache có sẵn để tái sử dụng
*/
func NewSearcherWithCache(items []string, cache *QueryCache) *Searcher {
	s := NewSearcher(items)
	if cache != nil {
		s.Cache = cache
	}
	return s
}

/*
- Hàm quan trọng nhất, kết hợp Fuzzy Search + Levenshtein + Cache Boost
- Có lẽ mình quên nói ở trên là ta phải dùng Rune
- Ví dụ như:
s := "Việt Nam"
fmt.Println(len(s))  // 12 bytes -> SAI (8 mới đúng)
-> Đúng ra ta phải dùng Rune
s := "Việt Nam"
runes := []rune(s)
fmt.Println(len(runes))  // 8 (đúng 8 ký tự)
- Ta cần đếm số ký tự, chứ không tính theo byte được
*/
func (s *Searcher) Search(query string) []string {
	queryNorm := core.Normalize(query)
	// đếm số ký tự, không phải byte
	queryLen := 0
	for range queryNorm {
		queryLen++
	}
	queryWords := strings.Fields(queryNorm)

	// Ví dụ: User từng search "main" và chọn main.go nhiều lần:
	// cacheBoosts = {"/a/main.go": 5000}
	var cacheBoosts map[string]int
	if s.Cache != nil {
		cacheBoosts = s.Cache.GetBoostScores(query)
	}

	queryPattern := []byte(queryNorm)

	// Search bằng Smith-Waterman Fuzzy Matcher (tự implement, không dependency)
	// Dùng parallel version nếu có nhiều files
	var matches []core.FuzzyMatch
	if len(s.Normalized) >= 1000 {
		matches = core.FuzzyFindParallel(queryPattern, s.Normalized)
	} else {
		matches = core.FuzzyFind(queryPattern, s.Normalized)
	}

	// Reuse pre-alloc flat array, chỉ reset những index đã ghi
	uniqueScores := s.scoreBuf
	dirtyIndices := make([]int, 0, len(matches)+50)
	// Defer cleanup: reset những ô đã dùng về sentinel
	defer func() {
		for _, idx := range dirtyIndices {
			uniqueScores[idx] = math.MinInt
		}
	}()

	// OPTIMIZATION: Chỉ tính word bonus cho top 30 results
	// countWordMatches rất chậm (gọi LevenshteinRatio), không nên chạy cho tất cả
	maxWordBonusCalc := 30
	if len(matches) < maxWordBonusCalc {
		maxWordBonusCalc = len(matches)
	}

	for i, m := range matches {
		if i < maxWordBonusCalc {
			// Word bonus tính trên tên file (không phải full path)
			wordMatches := core.CountWordMatches(queryWords, s.FilenamesOnly[m.Index])
			wordBonus := wordMatches * 800

			// FFF.nvim logic: Exact filename bonus
			filenameBonus := 0
			if m.Exact {
				filenameBonus += 400
			}
			if s.FilenamesOnly[m.Index] == queryNorm {
				filenameBonus += 1000 // Tuyệt đối khớp file
			}

			// FFF.nvim logic: Distance Penalty
			pathLenPenalty := len(s.Originals[m.Index]) / 5

			if uniqueScores[m.Index] == math.MinInt {
				dirtyIndices = append(dirtyIndices, m.Index)
			}
			uniqueScores[m.Index] = m.Score + wordBonus + filenameBonus - pathLenPenalty
		} else {
			if uniqueScores[m.Index] == math.MinInt {
				dirtyIndices = append(dirtyIndices, m.Index)
			}
			uniqueScores[m.Index] = m.Score - (len(s.Originals[m.Index]) / 5)
		}
	}

	// Ta tính điểm sai chính tả dựa trên Levenshtein
	// Tức là nếu user gõ "maain" hay "mian" thì ta vẫn tính điểm cho "main"
	// Threshold = (queryLen / 3) + 1: cho phép khoảng 1 lỗi mỗi 3 ký tự + 1 lỗi bonus
	// Minimum threshold = 3: query ngắn (2-5 ký tự) vẫn cần đủ độ linh hoạt để match
	// needLevenshtein := len(uniqueResults) < 20
	if queryLen > 1 {
		baseThreshold := (queryLen / 3) + 1
		if baseThreshold < 3 {
			baseThreshold = 3
		}

		for i, nameNorm := range s.FilenamesOnly {
			// Thay vì: runesName := []rune(nameNorm)
			// Ta kiểm tra độ dài bằng len() byte trước cho nhanh (sơ loại)
			if len(nameNorm) < queryLen {
				continue
			}

			// So sánh với phần đầu của filename
			targetStr1 := core.FastSubstring(nameNorm, queryLen)
			// Nếu sau khi cắt mà độ dài vẫn ngắn hơn query (do ký tự utf8) thì bỏ
			if len(targetStr1) < len(queryNorm) { // so sánh byte length ok vì đã normalized
				continue
			}

			dist := core.LevenshteinRatio(queryNorm, targetStr1)

			// So sánh thêm 1 ký tự (phòng trường hợp typo thêm ký tự)
			if len(nameNorm) > len(targetStr1) {
				// Lấy prefix dài hơn 1 rune
				targetStr2 := core.FastSubstring(nameNorm, queryLen+1)

				d2 := core.LevenshteinRatio(queryNorm, targetStr2)
				if d2 < dist {
					dist = d2
				}
			}
			/*
				Ở phần trên ví dụ như "mian", target 1 là "main" target 2 là "maina"
				Ta tính điểm ở target 1, dist = d1 = 2, nhưng ở target 2, dist = d2 = 3
				if d2 < dist {
						dist = d2
					}
				Tức là nếu nhỏ hơn cái d1 thì lấy, còn không thì giữ nguyên
				Kiểu như min(d1, d2)
			*/

			// Nếu điểm sai chính tả nhỏ hơn ngưỡng cho phép thì tính điểm
			// Robust solution khi sai chính tả đi quá xa (hoặc nếu không thì mong bạn có thể mở PR hỗ trợ mình)
			if dist <= baseThreshold {
				// Base score 3000
				score := 3000 - (dist * 400)
				runeCountName := 0
				for range nameNorm {
					runeCountName++
				}
				lenDiff := runeCountName - queryLen
				if lenDiff > 0 {
					score -= (lenDiff * 15) // Phạt độ dài tên
				}

				// Thưởng exact
				if lenDiff == 0 && dist == 0 {
					score += 1000
				}

				// Phạt độ dài đường dẫn
				score -= len(s.Originals[i]) / 5

				// Thêm word bonus cho Levenshtein matches
				if dist < 2 {
					wordMatches := core.CountWordMatches(queryWords, s.FilenamesOnly[i])
					score += wordMatches * 800
				}

				if uniqueScores[i] == math.MinInt {
					dirtyIndices = append(dirtyIndices, i)
				}
				if uniqueScores[i] == math.MinInt || score > uniqueScores[i] {
					uniqueScores[i] = score
				}
			}
		}
	}
	/*
		Đảm bảo file đã cache luôn xuất hiện trong kết quả, kể cả khi fuzzy/Levenshtein không match
		Thì ví dụ như:
		User search "tiền lương", xong họ chả chọn cái gì liên quan tới tiền lương
		nhưng chọn "bao_cao_tai_chinh_2024.xlsx"
		Hệ thống lưu lại: Query: "tiền lương" -> File: "bao_cao..."
		Xong giờ search lại "tien luong" một lần nữa
		Lúc này cả fuzzy và levenshtein đều không match
		Đoạn code này sẽ giải quyết vấn đề trên
		Nó vẫn in ra "bao_cao_tai_chinh_2024.xlsx", vì trước đây từng có hành vi này
		Và có thể nó sẽ là 1 trong những file user cần
		Đây chỉ là một cơ chế phòng bị cho trường hợp user quên tên file
		vì nó cũng không có độ chính xác quá cao
	*/
	for cachedPath, boost := range cacheBoosts {
		if idx, exists := s.FilePathToIdx[cachedPath]; exists {
			if uniqueScores[idx] == math.MinInt {
				dirtyIndices = append(dirtyIndices, idx)
				uniqueScores[idx] = boost
			}
		}
	}
	/*
		File: "/a/main.go"
		Fuzzy score: 85
		Cache boost: 5000
		Final score: 85 + 5000 = 5085 -> Lên top
	*/
	rankedResults := make([]MatchResult, 0, 100)
	for idx, score := range uniqueScores {
		if score == math.MinInt {
			continue
		}
		filePath := s.Originals[idx]
		finalScore := score

		if boost, exists := cacheBoosts[filePath]; exists {
			if score != boost {
				finalScore += boost
			}
		}

		rankedResults = append(rankedResults, MatchResult{
			Str:   filePath,
			Score: finalScore,
		})
	}
	// Logic:
	// Điểm cao lên trước
	// Cùng điểm, ưu tiên file path ngắn
	slices.SortFunc(rankedResults, func(a, b MatchResult) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		return len(a.Str) - len(b.Str)
	})
	// Trả về top 20, nếu kết quả ít hơn 20 thì show bấy nhiêu thôi
	// Hãy xem demo
	var results []string
	limit := 20
	if len(rankedResults) < limit {
		limit = len(rankedResults)
	}
	for _, res := range rankedResults[:limit] {
		results = append(results, res.Str)
	}
	return results
}

/*
- RecordSelection: Chỉ để gọi nhanh hơn, ngắn hơn
*/
func (s *Searcher) RecordSelection(query, filePath string) {
	if s.Cache != nil {
		s.Cache.RecordSelection(query, filePath)
	}
}

/*
- GetCache: Lấy object cache
*/
func (s *Searcher) GetCache() *QueryCache {
	return s.Cache
}

func (s *Searcher) ClearCache() {
	if s.Cache != nil {
		s.Cache.Clear()
	}
}

// Alias để đỡ phải import core, đơn giản là mình không muốn để các file ra source chính

type QueryCache = core.QueryCache
type CacheEntry = core.CacheEntry

func NewQueryCache() *QueryCache {
	return core.NewQueryCache()
}

func Normalize(s string) string {
	return core.Normalize(s)
}

func LevenshteinRatio(s1, s2 string) int {
	return core.LevenshteinRatio(s1, s2)
}
