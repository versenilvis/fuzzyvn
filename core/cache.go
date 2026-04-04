package core

import (
	"slices"
	"strings"
	"sync"
)

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
		dist := LevenshteinRatio(q1, q2)

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

	slices.SortFunc(matches, func(a, b fileScore) int {
		return b.score - a.score
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

	slices.SortFunc(files, func(a, b *fileInfo) int {
		if a.queryIndex != b.queryIndex {
			return b.queryIndex - a.queryIndex
		}
		return b.count - a.count
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
