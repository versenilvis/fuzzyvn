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
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/unicode/norm"
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
type CacheEntry struct {
	FilePath    string // Đường dẫn file đã chọn
	SelectCount int    // Số lần user chọn file này
}

type QueryCache struct {
	mu          sync.RWMutex            // Dùng RWMutex thay vì Mutex vì search chủ yếu là đọc (99%), tránh race codition
	entries     map[string][]CacheEntry // Key là từ khóa (đã chuẩn hóa), Value là danh sách các CacheEntry
	queryOrder  []string                // Danh sách lưu thứ tự các từ khóa đã tìm, cái nào lâu không dùng thì xóa trước
	maxQueries  int                     // Giới hạn tổng số từ khóa được lưu
	maxPerQuery int                     // Giới hạn số file được lưu cho mỗi từ khóa
	boostScore  int                     // Điểm cho các file hay search
}

type Searcher struct {
	Originals     []string       // Data gốc (có dấu, viết hoa thường lộn xộn bla bla). Dùng để trả về kết quả hiển thị
	Normalized    [][]rune       // Data đã chuẩn hóa cho fuzzy search, lưu dưới dạng rune để tìm kiếm nhanh
	FilenamesOnly []string       // Chỉ chứa tên file đã chuẩn hóa (bỏ đường dẫn). Dùng cho thuật toán Levenshtein (sửa lỗi chính tả)
	FilePathToIdx map[string]int // Nhằm mục đích không phải tạo lại mỗi lần Search
	Cache         *QueryCache    // Để lấy dữ liệu lịch sử
}

/*
- Struct tạm thời dùng để gom kết quả và điểm số lại để sắp xếp trước khi trả về cho người dùng
*/
type MatchResult struct {
	Str   string
	Score int
}

/*
FuzzyMatch: Kết quả của một fuzzy match
- Index: Vị trí trong danh sách input
- Score: Điểm match (cao = tốt hơn)
*/
type FuzzyMatch struct {
	Index int
	Score int
	Exact bool
}

var intSlicePool = sync.Pool{
	New: func() interface{} {
		s := make([]int, 0, 64)
		return &s
	},
}

// =============================================================================
// Utility Functions
// =============================================================================

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func isSeparator(r rune) bool {
	// Liệt kê các ký tự ngăn cách phổ biến trong code/path
	return r == '/' || r == '\\' || r == '_' || r == '-' || r == '.' || r == ' ' || r == ':'
}

func countWordMatches(queryWords []string, target string, levBuf *[]int) int {
	if len(target) < 2 {
		return 0
	}
	targetWords := strings.FieldsFunc(target, isSeparator)
	if len(targetWords) == 0 {
		return 0
	}
	count := 0
	for _, qWord := range queryWords {
		if len(qWord) < 2 {
			continue
		}
		for _, tWord := range targetWords {
			if len(tWord) < 2 {
				continue
			}
			// Exact match
			if qWord == tWord {
				count++
				break
			}
			// Fuzzy match: cho phép 1 lỗi nếu từ >= 3 ký tự
			// CHỈ check Levenshtein nếu độ dài gần bằng nhau
			if len(qWord) >= 3 && len(tWord) >= 3 && abs(len(qWord)-len(tWord)) <= 1 {
				dist := LevenshteinRatio(qWord, tWord, levBuf)
				if dist <= 1 {
					count++
					break
				}
			}
		}
	}
	return count
}

func Normalize(s string) string {
	// 1. FAST PATH: Nếu toàn là ASCII (Tiếng Anh, Code) -> Lowercase và trả về ngay
	// Đây là trường hợp phổ biến nhất (90% file source code) -> Tốc độ siêu nhanh
	isASCII := true
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			isASCII = false
			break
		}
	}
	if isASCII {
		return strings.ToLower(s)
	}

	// 2. NFD CHECK: Chỉ convert về NFC nếu chuỗi đang ở dạng NFD (thường gặp trên macOS)
	// Hàm IsNormalString rất nhanh, giúp tránh việc allocate lại chuỗi nếu không cần thiết
	if !norm.NFC.IsNormalString(s) {
		s = norm.NFC.String(s)
	}

	// 3. BUILDER: Dùng Builder để nối chuỗi hiệu quả
	var b strings.Builder
	// Grow đúng kích thước để tránh alloc nhiều lần.
	// Chuỗi không dấu thường ngắn hơn hoặc bằng chuỗi có dấu.
	b.Grow(len(s))

	// 4. MANUAL MAPPING: Duyệt từng rune và map thủ công
	// Lưu ý: Không dùng range strings.ToLower(s) để tránh tạo string tạm
	for _, r := range s {
		// Lowercase từng ký tự
		r = unicode.ToLower(r)

		switch r {
		case 'á', 'à', 'ả', 'ã', 'ạ', 'ă', 'ắ', 'ằ', 'ẳ', 'ẵ', 'ặ', 'â', 'ấ', 'ầ', 'ẩ', 'ẫ', 'ậ':
			b.WriteRune('a')
		case 'đ':
			b.WriteRune('d')
		case 'é', 'è', 'ẻ', 'ẽ', 'ẹ', 'ê', 'ế', 'ề', 'ể', 'ễ', 'ệ':
			b.WriteRune('e')
		case 'í', 'ì', 'ỉ', 'ĩ', 'ị':
			b.WriteRune('i')
		case 'ó', 'ò', 'ỏ', 'õ', 'ọ', 'ô', 'ố', 'ồ', 'ổ', 'ỗ', 'ộ', 'ơ', 'ớ', 'ờ', 'ở', 'ỡ', 'ợ':
			b.WriteRune('o')
		case 'ú', 'ù', 'ủ', 'ũ', 'ụ', 'ư', 'ứ', 'ừ', 'ử', 'ữ', 'ự':
			b.WriteRune('u')
		case 'ý', 'ỳ', 'ỷ', 'ỹ', 'ỵ':
			b.WriteRune('y')
		default:
			// Giữ lại các ký tự ASCII (a-z, 0-9, symbol) và các ký tự Unicode khác không phải tiếng Việt
			if r < 128 || unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func fastSubstring(s string, n int) string {
	if len(s) <= n {
		return s
	}

	count := 0
	/*
		Tại sao ta không dùng for i := 0; i < len(s); i++?
		Hàm len(s) trả về số lượng BYTE, không phải số lượng KÝ TỰ
		Ta có thể lợi dụng cách range hoạt động trong Go
		range s trong Go sẽ tự động nhảy theo TỪNG KÝ TỰ (Rune), không phải từng byte
	*/
	for i := range s {
		if count == n {
			// Tại thời điểm này, 'i' đang đứng đúng ở vị trí byte kết thúc ký tự thứ n
			// s[:i] là thao tác "Slice string"
			// Nó KHÔNG copy dữ liệu, nó chỉ trỏ vào vùng nhớ cũ
			return s[:i] // Cắt chuỗi tại byte index (Zero Alloc)
		}
		count++
	}
	/*
		Ví dụ:
		Chuỗi "âb" (giả sử â là 2 byte, b là 1 byte). Tổng 3 byte. Cần lấy 1 ký tự (n=1)
		Vòng lặp chạy lần 1: Đọc chữ â
		count tăng lên 1. count == n (1==1)
		Biến i lúc này đang ở vị trí byte tiếp theo (tức là byte số 2).
		return s[:2] -> Trả về đúng chữ â
	*/

	return s
}

func containsRunes(target []rune, pattern []rune) bool {
	if len(pattern) == 0 {
		return true
	}
	if len(pattern) > len(target) {
		return false
	}
	for i := 0; i <= len(target)-len(pattern); i++ {
		match := true
		for j := range pattern {
			if target[i+j] != pattern[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

/*
- Levenshtein Distance: https://viblo.asia/p/khoang-cach-levenshtein-va-fuzzy-query-trong-elasticsearch-jvElaOXAKkw
- Bạn hiểu nôm na là để tính độ sai lệch khi gõ sai, tìm kết quả gần khớp với ý muốn của bạn nhất
- Mính sẽ chỉ triển khai cái nào cần cho tiếng Việt thôi, Trung, Hàn, Nhật,... bỏ qua
- Mục tiêu là biến chuỗi s1 thành s2
- Tại mỗi bước so sánh ký tự, ta có 3 quyền lựa chọn, ta sẽ chọn cái nào tốn ít chi phí nhất (minVal):
+ Xóa bỏ ký tự ở s1 (Chi phí +1)
+ Thêm ký tự vào s1 để giống s2 (Chi phí +1)
+ Thay thế:
> Nếu 2 ký tự giống nhau: Không mất phí (+0)
> Nếu khác nhau: Thay ký tự này bằng ký tự kia (+1)
NOTE: 1 điều lưu ý là ta không cần quan tâm chữ hoa, chữ thường vì đã chuẩn hóa rồi
*/

func LevenshteinRatio(s1, s2 string, levBuf *[]int) int {
	/*
		Đây là trường hợp biến chuỗi s1 thành "chuỗi rỗng"
		Ví dụ s1 = "ABC", s2 = ""
		Biến "" thành "" mất 0 bước (column[0] = 0)
		Biến "A" thành "" mất 1 bước xóa (column[1] = 1)
		Biến "AB" thành "" mất 2 bước xóa (column[2] = 2)
		Lúc này mảng column trông như thế này: [0, 1, 2, 3, ... len(s1)]
		Tương ứng tăng dần từ 0 đến len(s1) là chi phí biến thành chuỗi rỗng
	*/
	s1Len := len(s1)
	s2Len := len(s2)

	/*
		Cái này hiểu đơn giản là
		Ví dụ như: "" -> "ABC" thì ta lấy luôn độ dài s2
		Ngược lại "ABC" -> "" thì lấy độ dài s1
		Vì rõ ràng số bước thay đổi từ rỗng thành text, hay text thành rỗng tốn số bước
		đúng bằng độ dài của nó
		Điều này giúp ta bỏ qua mấy bước bên dưới, làm tốn thêm phép toán và chậm đi chương trình
	*/
	if s1Len == 0 {
		return s2Len
	}
	if s2Len == 0 {
		return s1Len
	}
	// Thay vì cấp phát mới (make) mỗi lần gọi, ta mượn slice từ Pool
	// Giúp giảm allocation
	column := *levBuf
	// Kiểm tra sức chứa của slice mượn được
	// Nếu slice mượn được quá bé không đủ chứa (s1Len + 1),
	// bắt buộc phải cấp phát vùng nhớ mới to hơn.
	if cap(column) < s1Len+1 {
		column = make([]int, s1Len+1)
	}
	// Resize lại độ dài slice đúng bằng nhu cầu sử dụng
	column = column[:s1Len+1]
	// IMPORTAN: Trả lại slice về Pool sau khi tính toán xong.
	// Phải gán lại *ptr = column phòng trường hợp slice bị tạo mới (re-allocated)
	defer func() {
		*levBuf = column
	}()

	for y := 1; y <= s1Len; y++ {
		column[y] = y
	}
	/*
			Ở đây mình sẽ giải thích sơ sơ
			Thay vì dùng ma trận, mình dùng column như 1 stack từ trên xuống vậy, và ta sẽ ghi đè lên cái nào đã dùng
			Chủ yếu để tiết kiệm 1 chút bộ nhớ thôi
			Giờ nhìn ma trận trước
		        /*
		          "" |  A |  B |  C
		        ┌────┬────┬────┬────┐
		      ""│  0 │  1 │  2 │  3 │   ← khởi tạo, từ rỗng thành rỗng cần 0 bước, thành A cần qua chữ A, thành B cần qua A,B, thành C cần qua A,B,C
		        ├────┼────┼────┼────┤
		      A │  1 │  0 │  1 │  2 │   ← A=A (0), còn lại +1 theo cách biến đổi như cách khởi tạo
		        ├────┼────┼────┼────┤
		      X │  2 │  1 │  ? │  2 │   ← chuỗi AX, đổi thành rỗng cần 2 bước,... nhưng X≠B đọc tiếp xuống dưới
		        ├────┼────┼────┼────┤
		      C │  3 │  2 │  2 │  1 │   ← Tương tự, tại ô của B, Biến AX thành AB (tốn 1 bước sửa X->B), sau đó dư chữ C nên phải Xóa C (1 bước nữa)
		        └────┴────┴────┴────┘
			Kết quả tại ô "?" = 1
			Vì ô ? = min(trên, trái, chéo trái) + 1 (+1 khi ta thấy được ký tự khác nhau)
			Còn bạn nhìn vào ô (4,4) (C,C) ta thấy nó bằng 1 vì min(trên, trái, chéo trái) không + 1 vì C-C giống nhau
			Bây giờ, hãy xem chuyện gì xảy ra khi ta ép cái bảng trên vào 1 mảng duy nhất (column)

	*/

	for i := 1; i <= s2Len; i++ {
		column[0] = i    // Ví dụ: "" -> "A" (1 thêm), "" -> "AX" (2 thêm)
		lastKey := i - 1 // Lưu giá trị cũ của ô chéo trên trái ta đã đề cập
		for j := 1; j <= s1Len; j++ {
			/*
					IMPORTANT: Lưu lại giá trị cũ của column[j] trước khi bị ghi đè
					column[j] lúc này đang chứa giá trị của hàng bên trên (i-1)
					Sau khi vòng lặp này kết thúc, giá trị này sẽ trở thành
				    ô chéo trên trái cho vòng lặp tiếp theo (j+1)
			*/
			oldKey := column[j]
			/*
							Tính toán chi phí biến đổi:

									(lastKey)    (column[j] cũ)
				   					  CHÉO      |     TRÊN
				    				   ↘        |      ↓
				           					┌───────┐
				  				   TRÁI ──→ │  ???  │  (Đang tính)
				              (column[j-1]) └───────┘

							NOTE: lastKey = column[j-1]
			*/
			var incr int
			if s1[j-1] != s2[i-1] {
				incr = 1 // Khác nhau: +1 bước thay thế, còn không thì thôi không cần cộng
			}

			// Và đây chính xác là cái min chúng ta đã làm ở trên: min(trên, trái, chéo trái)
			// Xóa. Ví dụ: Name -> Nam
			minVal := column[j] + 1
			// Thêm. Ví dụ: Nam -> Name
			if column[j-1]+1 < minVal {
				minVal = column[j-1] + 1
			}
			// Sửa. Ví dụ: Năm -> Nấm
			if lastKey+incr < minVal {
				minVal = lastKey + incr
			}
			column[j] = minVal
			// Giá trị Trên của ô hiện tại (oldKey) sẽ trở thành
			// giá trị Chéo của ô bên phải
			lastKey = oldKey
		}
	}
	// Trả về chi phí cuối dựa trên độ dài s1 (phần tử cuối). Đọc tới đây mà không hiểu thì hãy xem lại ma trận
	return column[s1Len]
}

// =============================================================================
// Fuzzy Matcher
// =============================================================================

/*
fuzzyScoreGreedy: Tính điểm fuzzy match sử dụng thuật toán tham lam
- pattern: Query đã normalize
- target: Target string đã normalize
- Trả về: (matched bool, score int, positions []int)
- Cách này có 1 vấn đề nho nhỏ, là do tham lam
- Nó có thể bỏ qua một match tốt hơn ở sau để chọn match đầu tiên tìm được
- Nhưng bù lại cực nhanh vì chỉ duyệt target 1 lần
*/
func fuzzyScoreGreedy(pattern []rune, target []rune) (int, bool) {
	lenP := len(pattern)
	lenT := len(target)

	if lenP > lenT {
		return 0, false
	}

	totalScore := 0

	// Index của ký tự khớp trước đó trong target
	prevMatchIdx := -1

	// Index hiện tại đang xét trong target
	targetIdx := 0

	for pIdx := range lenP {
		pChar := pattern[pIdx]
		found := false

		bestScore := -1
		bestIdx := -1

		// GREEDY LOOK-AHEAD:
		// Thay vì lấy ngay ký tự tìm thấy đầu tiên, ta quét hết phần còn lại
		// để tìm xem có ký tự nào ngon hơn không (ví dụ: đầu từ)
		// Tuy nhiên quét hết thì chậm O(n*m).
		// Ta dùng chiến thuật: Tìm ký tự đầu tiên -> Lưu lại
		// Nếu nó KHÔNG PHẢI đầu từ, thì ráng tìm tiếp xem có cái nào là đầu từ không

		for t := targetIdx; t < lenT; t++ {
			tChar := target[t]

			if tChar == pChar {
				// Tính điểm sơ bộ cho vị trí này
				score := 0

				// Thưởng đầu từ
				// Ký tự là đầu từ nếu: nó là ký tự đầu tiên OR ký tự trước nó là dấu ngăn cách
				isWordStart := false
				if t == 0 {
					isWordStart = true
				} else {
					prevChar := target[t-1]
					// Check separator: dấu cách, _, -, /, .
					if isSeparator(prevChar) {
						isWordStart = true
					} else if unicode.IsLower(prevChar) && unicode.IsUpper(tChar) {
						// CamelCase (aB) -> B là đầu từ
						isWordStart = true
					}
				}

				if isWordStart {
					score += 80 // Thưởng đậm cho đầu từ
				} else {
					score += 10 // Điểm cơ bản
				}

				// Thưởng liền kề
				if prevMatchIdx != -1 && t == prevMatchIdx+1 {
					score += 40 // Thưởng cho việc gõ liền mạch
				}

				// Phạt khoảng cách
				// Logic: Càng xa ký tự trước càng trừ điểm
				/* Phần này làm Greedy phức tạp, ở đây ta đơn giản hóa:
				   Nếu tìm thấy WordStart -> CHỐT LUÔN (Greedy lấy cái tốt nhất)
				   Nếu tìm thấy ký tự thường -> Lưu tạm, tìm tiếp xem có WordStart không
				*/

				if isWordStart {
					bestScore = score
					bestIdx = t
					found = true
					break // Tìm thấy đầu từ rồi, lấy luôn không cần tìm nữa
				}

				// Nếu chưa có
				if bestIdx == -1 {
					bestScore = score
					bestIdx = t
					found = true
				}
			}
		}

		if !found {
			return 0, false // Không tìm thấy ký tự pattern
		}

		// Chốt phương án cho ký tự pattern này
		totalScore += bestScore
		prevMatchIdx = bestIdx

		// Ký tự tiếp theo của pattern phải tìm sau vị trí này
		targetIdx = bestIdx + 1
	}

	// Penalty nếu độ dài chênh lệch nhiều
	lenDiff := lenT - lenP
	if lenDiff > 0 {
		totalScore -= lenDiff * 2
	}

	return totalScore, true
}

/*
FuzzyFind: Tìm tất cả targets khớp với pattern
- pattern: Query string (đã lowercase + normalize)
- targets: Danh sách strings để search
- Duyệt qua từng target trong danh sách targets
- Gọi fuzzyScoreGreedy cho từng cặp (pattern, target)
- Nếu match thì thêm vào results (Index, Score, Positions)
- Xong sort theo score giảm dần
*/
func FuzzyFind(pattern string, targets [][]rune) []FuzzyMatch {
	patternRunes := []rune(Normalize(pattern)) // 1 alloc
	if len(patternRunes) == 0 {
		return nil
	}
	// Pre-allocate slice kết quả để tránh resize liên tục
	results := make([]FuzzyMatch, 0, 1000)

	for idx := range targets {
		score, matched := fuzzyScoreGreedy(patternRunes, targets[idx])

		if matched {
			exact := containsRunes(targets[idx], patternRunes)
			if exact {
				score += 200
			}

			results = append(results, FuzzyMatch{
				Index: idx,
				Score: score,
				Exact: exact,
			})
		}
	}
	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

/*
FuzzyFindParallel: Version parallel của FuzzyFind
- OK giờ bạn sẽ thắc mắc như này: "Tại sao lại cần FuzzyFind khi đã có parrallel version?"
- Lý do chính là để giảm thiểu chi phí overhead khi xử lý các tập dữ liệu nhỏ
- Bởi vậy nên đoạn ở dưới mới có if numTargets < 1000 thì dùng FuzzyFind đó
- Dưới 1000 files dùng FuzzyFind thay vì FuzzyFindParallel để tránh overhead, vẫn đảm bảo tốc độ
Sử dụng goroutines để tăng tốc với datasets lớn
- pattern: Query string
- targets: Danh sách strings để search
- Trả về: Slice of FuzzyMatch, sorted by score descending
*/
func FuzzyFindParallel(pattern string, targets [][]rune) []FuzzyMatch {
	patternRunes := []rune(pattern)
	if len(patternRunes) == 0 {
		return nil
	}

	numTargets := len(targets)
	// Chỉ dùng parallel nếu dataset lớn
	if numTargets < 2000 {
		return FuzzyFind(pattern, targets)
	}

	/*
		Thường đúng ra thì để tận dụng tối đa nên dùng công thức: workers = tổng số luồng
		Ví dụ: 4 nhân, 4 luồng -> 16 workers
		Nhưng mà trong thực tế thì không phải lúc nào cũng tận dụng tối đa
		Nên ta chỉ dùng 16 workers
		Vì dùng ít hơn thì lãng phí, còn dùng nhiều hơn thì overhead
		Nhưng mà cho dù số luồng nhiều hơn nữa như 32, 64 vẫn nên dùng max là 16 thôi vì overhead lúc này cao hơn lợi ích mang lại
	*/
	numWorkers := runtime.NumCPU()
	if numWorkers > 16 {
		numWorkers = 16
	}
	/*
		Trong chia số nguyên của Go nó bị làm tròn xuống, ví dụ 10 việc mà chia 3 người, sẽ là 3 việc mỗi người
		Ta sẽ bị thiếu đi 1 việc thứ 10
		Chúng ta muốn: Nếu chia không hết, thì mỗi người phải gánh thêm một chút để đảm bảo không bỏ sót việc nào
		Tức là 10 / 3 phải bằng 4, chứ không phải 3
		Và ta có công thức làm tròn lên (A+B-1)/B
		Và giờ bạn sẽ thắc mắc, thế còn 9 khi chia hết?
		Cũng như trên, ta có (9+3-1)/3 = 11/3 = 3 vì ta lợi dụng lại phép chia số nguyên Go như ta đã nói sẽ tự làm tròn xuống thành 3 cho dù 3.66
		Và ra 3 thì vẫn chia đúng việc 3 người

		Ví dụ:
		a10 := 10
		a9 := 9
		b := 3
		fmt.Println((a10 + b - 1) / 3) // 4
		fmt.Println((a9 + b - 1) / 3) // 3

		Có thể bạn sẽ thắc mắc thêm chia việc cho các workers ở chunksizes = 4
		Khi đó A -> làm 4 việc
		B -> làm 4 việc
		C -> làm 2 việc
		Thật ra C phải làm 4 nhưng ta đã handle bằng cách cho end = numTargets khi end > numTargets (đoạn code for bên dưới)
		Vậy nên làm 2 việc thôi, và tổng vẫn 10
		Với 9 việc thì chia 3 vẫn ra 3 nên không có gì xảy ra
	*/
	chunkSize := (numTargets + numWorkers - 1) / numWorkers
	resultChan := make(chan []FuzzyMatch, numWorkers)

	var wg sync.WaitGroup
	for w := range numWorkers {
		start := w * chunkSize
		end := start + chunkSize
		if end > numTargets {
			end = numTargets
		}
		if start >= numTargets {
			break
		}

		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			localResults := make([]FuzzyMatch, 0, (end-start)/5)

			for i := start; i < end; i++ {
				score, matched := fuzzyScoreGreedy(patternRunes, targets[i])

				if matched {
					exact := containsRunes(targets[i], patternRunes)
					if exact {
						score += 200
					}

					localResults = append(localResults, FuzzyMatch{
						Index: i,
						Score: score,
						Exact: exact,
					})
				}
			}

			resultChan <- localResults
		}(start, end)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results từ từng worker
	allResults := make([]FuzzyMatch, 0, 1000)
	for localResults := range resultChan {
		allResults = append(allResults, localResults...)
	}

	// Sắp xếp kết quả theo score giảm dần
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})

	return allResults
}

// =============================================================================
// QueryCache Internal Methods (Helpers)
// =============================================================================

/*
- querySimilarity: chấm điểm độ tương đồng giữa hai câu truy vấn (q1 và q2) trên thang điểm từ 0 đến 100
- Mục đích là để xem q1 (câu người dùng mới nhập) có đủ giống với q2 (câu đã lưu trong cache) hay không để tận dụng kết quả cũ
- Hàm này hoạt động theo cơ chế "Sàng lọc theo tầng" (Layered Filtering), ưu tiên độ chính xác từ cao xuống thấp
*/
func (c *QueryCache) querySimilarity(q1, q2 string) int {
	if q1 == q2 {
		return 100
	}
	// Ví dụ: "samsung" (q1), cache có "samsung s23" (q2)
	if strings.HasPrefix(q2, q1) {
		return 70 + (30 * len(q1) / len(q2))
	}

	if strings.HasPrefix(q1, q2) && len(q2) >= 2 {
		return 50 + (30 * len(q2) / len(q1))
	}
	// Ví dụ: q1="ip 15", q2="mua ip 15 giá rẻ"
	if len(q1) >= 2 && strings.Contains(q2, q1) {
		return 80
	}
	// Ví dụ: q1="mua ip 15 giá rẻ", q2="ip 15"
	if len(q2) >= 2 && strings.Contains(q1, q2) {
		return 60
	}
	// Bạn hoàn toàn có thể thay đổi cơ chế chấm điểm bên trên

	// Nếu không khớp chuỗi liền mạch, hàm sẽ cắt chuỗi thành từng từ (bằng strings.Fields) để so sánh
	// Logic này xử lý việc đảo từ
	words1 := strings.Fields(q1)
	words2 := strings.Fields(q2)
	// Tách từ ra, tìm xem có bao nhiêu từ giống nhau
	if len(words1) > 0 && len(words2) > 0 {
		commonWords := 0
		for _, w1 := range words1 {
			for _, w2 := range words2 {
				if w1 == w2 && len(w1) >= 2 {
					commonWords++
					break
				}
			}
		}
		/*
			Ví dụ:
			q1: "sơn tùng mtp"
			q2: "mtp sơn tùng"
			Hai chuỗi này Contains sẽ sai, nhưng tách từ thì khớp 3 từ.
			Điểm: 50 + (15 điểm cho mỗi từ trùng). Nếu trùng 3 từ = 95 điểm
			Nếu điểm cao như này thì hoàn toàn khẳng định được đây là từ khóa cần tìm
		*/
		if commonWords > 0 {
			return 50 + (commonWords * 15)
		}
	}

	/*
		Sai chính tả
	*/
	if len(q1) >= 3 && len(q2) >= 3 {
		// Tính khoảng cách Levenshtein
		levBuf := intSlicePool.Get().(*[]int)
		dist := LevenshteinRatio(q1, q2, levBuf)
		intSlicePool.Put(levBuf)

		maxLen := len(q1)
		if len(q2) > maxLen {
			maxLen = len(q2)
		}
		// Tính ngưỡng sai số cho phép (threshold): Khoảng 30% độ dài chuỗi dài nhất
		// Ví dụ chuỗi dài 10 ký tự thì cho phép sai tối đa 3 lỗi
		threshold := maxLen * 30 / 100
		if threshold < 2 {
			threshold = 2
		}
		/*
			Nếu số lỗi nằm trong ngưỡng cho phép: Trả về 60 trừ đi điểm phạt (mỗi lỗi trừ 10 điểm)
			Ví dụ:
			q1: "iphone"
			q2: "ipbone" (Sai 1 ký tự h -> b, dist = 1)
			Điểm: 60 - (1 * 10) = 50 điểm
		*/
		if dist <= threshold {
			return 60 - (dist * 10)
		}
	}

	return 0
}

//
/*
- moveToFront: Đẩy query lên đầu danh sách queryOrder
- Để query được tìm kiếm nhiều nhất sẽ được ưu tiên hơn
- Nhưng mà trong code ta đẩy xuống cuối mảng
*/
func (c *QueryCache) moveToFront(query string) {
	for i, q := range c.queryOrder {
		if q == query {
			c.queryOrder = append(c.queryOrder[:i], c.queryOrder[i+1:]...)
			break
		}
	}
	c.queryOrder = append(c.queryOrder, query)
}

/*
- evictIfNeeded: Xóa query cũ nhất nếu vượt giới hạn maxQueries
*/
func (c *QueryCache) evictIfNeeded() {
	for len(c.queryOrder) > c.maxQueries {
		oldestQuery := c.queryOrder[0]
		c.queryOrder = c.queryOrder[1:]
		delete(c.entries, oldestQuery)
	}
}

// =============================================================================
// QueryCache Public Methods
// =============================================================================

// NewQueryCache: Define một object QueryCache mặc định, có thể tùy chỉnh dựa tùy vào project của bạn
func NewQueryCache() *QueryCache {
	return &QueryCache{
		entries:     make(map[string][]CacheEntry),
		queryOrder:  make([]string, 0),
		maxQueries:  100,
		maxPerQuery: 5,
		boostScore:  5000,
	}
}

// SetMaxQueries: Đặt giới hạn tổng số từ khóa được lưu
// Sau khi giảm maxQueries, nó gọi evictIfNeeded để xóa bớt dữ liệu thừa ngay lập tức
func (c *QueryCache) SetMaxQueries(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxQueries = n
	c.evictIfNeeded()
}

// SetBoostScore: Đặt điểm boost cơ bản cho kết quả từ cache
func (c *QueryCache) SetBoostScore(score int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.boostScore = score
}

/*
- User search "main" -> Thấy danh sách files -> Click chọn "/project/main.go"
-> RecordSelection("main", "/project/main.go")
- NOTE: Hãy nhớ rằng bạn có thể định nghĩa thế nào là "chọn", hiện tại mình chỉ định nghĩa theo phần demo ở thư mục demo
*/
func (c *QueryCache) RecordSelection(query, filePath string) {
	if query == "" || filePath == "" {
		return
	}
	// Dùng lock vì hàm này cần ghi
	c.mu.Lock()
	defer c.mu.Unlock()
	// Chuẩn hóa query, xóa hết dấu, xóa hết các ký tự viết hoa
	// Ví dụ: Cộng đồng Golang Việt Nam -> cong dong golang viet nam
	queryNorm := Normalize(query)

	// Phải ưu tiên kiểm tra trong cache trước rồi mới tới các bước tiếp theo
	entries, exists := c.entries[queryNorm]
	// Case 1: Nếu có -> tăng count lên 1 và đẩy nó lên
	if exists {
		for i, entry := range entries {
			if entry.FilePath == filePath {
				c.entries[queryNorm][i].SelectCount++
				c.moveToFront(queryNorm)
				return
			}
		}
	}

	// Case 2: Nếu không có -> tạo mới một CacheEntry với count = 1
	newEntry := CacheEntry{FilePath: filePath, SelectCount: 1}
	if !exists {
		c.entries[queryNorm] = []CacheEntry{newEntry}
		c.queryOrder = append(c.queryOrder, queryNorm)
	} else { // Case 3: Đã có query nhưng mà file đó ta chưa thêm vào
		// Nếu đã đạt giới hạn số file được lưu cho mỗi từ khóa, xóa file có count thấp nhất
		if len(entries) >= c.maxPerQuery {
			minIdx := 0
			minCount := entries[0].SelectCount
			for i, e := range entries {
				if e.SelectCount < minCount {
					minCount = e.SelectCount
					minIdx = i
				}
			}
			// Xóa file có count thấp nhất
			c.entries[queryNorm] = append(entries[:minIdx], entries[minIdx+1:]...)
		}
		// Thêm file mới vào
		c.entries[queryNorm] = append(c.entries[queryNorm], newEntry)
	}

	c.moveToFront(queryNorm) // Đẩy query lên đầu danh sách
	c.evictIfNeeded()
}

/*
- GetBoostScores: Lấy điểm boost cho từng file dựa trên query người dùng
- Ví dụ như ta có query "màn hình"
- List ra sản phẩm có Màn hình Dell Ultrasharp hoặc chỉ đơn giản Dell Ultrasharp thôi, ...
- Nó sẽ học hành vi người dùng nhấn vào ví dụ Dell Ultrasharp mặc dù chả có cái chữ "màn hình" nào ở đây cả
- Nhưng nó vẫn sẽ lưu lại, càng nhiều lần càng cộng điểm
*/
func (c *QueryCache) GetBoostScores(query string) map[string]int {
	// Đoạn này dùng RLock vì chỉ đọc là chính, cho phép nhiều luồng (goroutine) cùng đọc một lúc
	// Điều này giúp hiệu năng cao hơn nhiều so với Lock thường (chỉ cho 1 người vào, dù chỉ để đọc)
	// Nói chung bạn hiểu nôm na là để handle nhiều query cùng một lúc
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]int)
	if query == "" {
		return result
	}

	queryNorm := Normalize(query)
	/*
		Kết quả nào càng giống ý định tìm kiếm VÀ càng được chọn nhiều trước đây, thì điểm cộng càng cao
		Dựa vào config boost cơ bản của bạn
		Độ giống nhau dựa vào querySimilarity
		Độ phổ biến dựa vào entry.SelectCount, kiểu như 1 người bấm vào chọn nhiều lần hoặc nhiều người bấm vào chọn
	*/
	for cachedQuery, entries := range c.entries {
		similarity := c.querySimilarity(queryNorm, cachedQuery)
		if similarity > 0 {
			for _, entry := range entries {
				boost := (c.boostScore * similarity * entry.SelectCount) / 100
				/*
									Một file (entry.FilePath) có thể xuất hiện trong nhiều cached query khác nhau
					    			Ví dụ: File "iPhone 15.html" xuất hiện khi tìm "iphone" và cả khi tìm "apple" (đại loại vậy)
									Đoạn code này đảm bảo: Nếu một file được tìm thấy nhiều lần,
									ta chỉ giữ lại điểm Boost cao nhất mà nó đạt được
				*/
				if currentBoost, exists := result[entry.FilePath]; !exists || boost > currentBoost {
					result[entry.FilePath] = boost
				}
			}
		}
	}

	return result
}

/*
- GetRecentQueries: Lấy danh sách query gần đây nhất
- Ví dụ như ta có query "màn hình"
- List ra các query gần đây nhất, ví dụ: "màn hình", "màn hình dell", "màn hình dell ultasharp", ...
- Hãy xem demo để biết
*/
func (c *QueryCache) GetRecentQueries(limit int) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if limit <= 0 || len(c.queryOrder) == 0 {
		return []string{}
	}

	result := make([]string, 0, limit)
	for i := len(c.queryOrder) - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, c.queryOrder[i])
	}
	return result
}

/*
- GetCachedFiles: Lấy danh sách file đã lưu trong cache
- Ví dụ như ta có query "màn hình"
- List ra các file đã lưu trong cache như /data/products/dell/dell-ultrasharp.html
- Hãy xem demo để biết
*/
func (c *QueryCache) GetCachedFiles(query string, limit int) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if query == "" || limit <= 0 {
		return []string{}
	}

	queryNorm := Normalize(query)

	type fileScore struct {
		path  string
		score int
	}
	var matches []fileScore
	seen := make(map[string]bool)

	// Ưu tiên cao nhất cho những query đã từng được gõ y hệt
	if entries, ok := c.entries[queryNorm]; ok {
		for _, entry := range entries {
			// Điểm cơ bản cực cao (100) * số lần click
			score := 100 * entry.SelectCount
			matches = append(matches, fileScore{path: entry.FilePath, score: score})
			seen[entry.FilePath] = true
		}
	}

	// Tìm các query liên quan khác. Ví dụ: gõ "màn hình", tìm thấy cả trong lịch sử "màn hình dell"
	for cachedQuery, entries := range c.entries {
		// Bỏ qua nếu là chính nó (đã xử lý ở trên)
		if cachedQuery == queryNorm {
			continue
		}

		// Nếu độ dài chuỗi lệch nhau quá 5 ký tự, khả năng cao là không liên quan -> Bỏ qua để đỡ tốn tài nguyên
		if abs(len(cachedQuery)-len(queryNorm)) > 5 {
			continue
		}

		similarity := c.querySimilarity(queryNorm, cachedQuery)
		if similarity > 0 {
			for _, entry := range entries {
				// Nếu đã có trong phần tìm khớp rồi thì không add lại
				if seen[entry.FilePath] {
					continue
				}
				// Điểm = Độ giống * Độ phổ biến
				score := similarity * entry.SelectCount
				matches = append(matches, fileScore{path: entry.FilePath, score: score})
			}
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	result := make([]string, 0, limit)

	// Reset map seen để dùng cho việc filter kết quả trả về
	seenResult := make(map[string]bool)

	for _, m := range matches {
		if !seenResult[m.path] {
			seenResult[m.path] = true
			result = append(result, m.path)
			if len(result) >= limit {
				break
			}
		}
	}

	return result
}

/*
- GetAllRecentFiles: Lấy lịch sử danh sách file đã lưu trong cache
- List ra các file đã lưu trong cache như /data/products/dell/dell-ultrasharp.html,...
- Màn hình chính, input rỗng -> hiển thị "Recent Files"
- Hãy xem demo để biết
*/
func (c *QueryCache) GetAllRecentFiles(limit int) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if limit <= 0 {
		return []string{}
	}

	type fileInfo struct {
		path       string
		queryIndex int
		count      int
	}
	fileMap := make(map[string]*fileInfo)

	for i, query := range c.queryOrder {
		entries := c.entries[query]
		for _, entry := range entries {
			if existing, ok := fileMap[entry.FilePath]; ok {
				if i > existing.queryIndex {
					existing.queryIndex = i
				}
				existing.count += entry.SelectCount
			} else {
				fileMap[entry.FilePath] = &fileInfo{
					path:       entry.FilePath,
					queryIndex: i,
					count:      entry.SelectCount,
				}
			}
		}
	}

	files := make([]*fileInfo, 0, len(fileMap))
	for _, f := range fileMap {
		files = append(files, f)
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].queryIndex != files[j].queryIndex {
			return files[i].queryIndex > files[j].queryIndex
		}
		return files[i].count > files[j].count
	})

	result := make([]string, 0, limit)
	for i := 0; i < len(files) && i < limit; i++ {
		result = append(result, files[i].path)
	}

	return result
}

/*
- Size: Lấy số lượng query đã lưu trong cache
*/
func (c *QueryCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

/*
- Clear: Xóa tất cả query đã lưu trong cache
*/
func (c *QueryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string][]CacheEntry)
	c.queryOrder = make([]string, 0)
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
	normPaths := make([][]rune, len(items))
	normNames := make([]string, len(items))
	pathMap := make(map[string]int, len(items))

	for i, item := range items {
		originals[i] = item
		filename := filepath.Base(item)
		// Ưu tiên tên file, theo path thì điểm thấp hơn
		priorityString := filename + " " + item
		normPaths[i] = []rune(Normalize(priorityString))
		normNames[i] = Normalize(filename)

		// Map trong cache để sau này server tìm trong các file gốc nhanh hơn
		pathMap[item] = i
	}

	return &Searcher{
		Originals:     items,
		Normalized:    normPaths,
		FilenamesOnly: normNames,
		FilePathToIdx: pathMap,
		Cache:         NewQueryCache(),
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
	queryNorm := Normalize(query)
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

	// Search bằng Smith-Waterman Fuzzy Matcher (tự implement, không dependency)
	// Dùng parallel version nếu có nhiều files
	var matches []FuzzyMatch
	if len(s.Normalized) >= 1000 {
		matches = FuzzyFindParallel(queryNorm, s.Normalized)
	} else {
		matches = FuzzyFind(queryNorm, s.Normalized)
	}

	// Ước lượng capacity là để hạn chế resize
	// Capacity cố định 1000 thay vì map resize bằng len matches khổng lồ
	uniqueResults := make(map[int]int, 1000)

	// OPTIMIZATION: Chỉ tính word bonus cho top 30 results
	// countWordMatches rất chậm (gọi LevenshteinRatio), không nên chạy cho tất cả
	maxWordBonusCalc := 30
	if len(matches) < maxWordBonusCalc {
		maxWordBonusCalc = len(matches)
	}

	// Lấy buffer Levenshtein dùng chung
	levBuf := intSlicePool.Get().(*[]int)
	defer intSlicePool.Put(levBuf)

	for i, m := range matches {
		if i < maxWordBonusCalc {
			// Word bonus tính trên tên file (không phải full path)
			wordMatches := countWordMatches(queryWords, s.FilenamesOnly[m.Index], levBuf)
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

			uniqueResults[m.Index] = m.Score + wordBonus + filenameBonus - pathLenPenalty
		} else {
			// Với results còn lại, chỉ dùng fuzzy score
			uniqueResults[m.Index] = m.Score - (len(s.Originals[m.Index]) / 5)
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
			targetStr1 := fastSubstring(nameNorm, queryLen)
			// Nếu sau khi cắt mà độ dài vẫn ngắn hơn query (do ký tự utf8) thì bỏ
			if len(targetStr1) < len(queryNorm) { // so sánh byte length ok vì đã normalized
				continue
			}

			dist := LevenshteinRatio(queryNorm, targetStr1, levBuf)

			// So sánh thêm 1 ký tự (phòng trường hợp typo thêm ký tự)
			if len(nameNorm) > len(targetStr1) {
				// Lấy prefix dài hơn 1 rune
				targetStr2 := fastSubstring(nameNorm, queryLen+1)

				d2 := LevenshteinRatio(queryNorm, targetStr2, levBuf)
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
					wordMatches := countWordMatches(queryWords, s.FilenamesOnly[i], levBuf)
					score += wordMatches * 800
				}

				if oldScore, exists := uniqueResults[i]; !exists || score > oldScore {
					uniqueResults[i] = score
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
		// Tra cứu trực tiếp từ map đã pre-compute
		if idx, exists := s.FilePathToIdx[cachedPath]; exists {
			if _, alreadyInResults := uniqueResults[idx]; !alreadyInResults {
				uniqueResults[idx] = boost
			}
		}
	}
	/*
		File: "/a/main.go"
		Fuzzy score: 85
		Cache boost: 5000
		Final score: 85 + 5000 = 5085 -> Lên top
	*/
	rankedResults := make([]MatchResult, 0, len(uniqueResults))
	for idx, score := range uniqueResults {
		filePath := s.Originals[idx]
		finalScore := score

		if boost, exists := cacheBoosts[filePath]; exists {
			if score != boost { // Tránh duplicate
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
	sort.SliceStable(rankedResults, func(i, j int) bool {
		if rankedResults[i].Score == rankedResults[j].Score {
			return len(rankedResults[i].Str) < len(rankedResults[j].Str)
		}
		return rankedResults[i].Score > rankedResults[j].Score
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
- Ví dụ:
cache := searcher.GetCache()
cache.GetRecentQueries(5)
cache.GetAllRecentFiles(10)
cache.SetMaxQueries(50)
*/
func (s *Searcher) GetCache() *QueryCache {
	return s.Cache
}

func (s *Searcher) ClearCache() {
	if s.Cache != nil {
		s.Cache.Clear()
	}
}
