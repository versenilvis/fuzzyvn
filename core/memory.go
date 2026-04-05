package core

import (
	"bytes"
	"encoding/gob"
	"math"
	"sort"
	"sync"
	"time"
)

/*
FileRecord: Lưu trữ lịch sử hành vi chọn file của người dùng
*/
type FileRecord struct {
	SelectCount int      // Tổng số lần file này được chọn
	LastAccess  int64    // Unix timestamp của lần chọn cuối cùng
	Queries     []string // Danh sách các query gần đây nhất dẫn tới việc chọn file này (lưu tối đa 3)
}

type FileMemory struct {
	mu            sync.RWMutex
	files         map[string]*FileRecord // Key là đường dẫn file tuyệt đối
	maxFiles      int                    // Giới hạn số lượng file được ghi nhớ (mặc định 500)
	decayHalfLife int64                  // Thời gian bán rã của điểm số (ví dụ 12 giờ)
	// decayHalfLife giải quyết bài toán sau
	/*
		File A: Bạn mở 100 lần, nhưng lần cuối là từ 1 tháng trước
		File B: Bạn mới mở 5 lần trong hôm nay
		nếu không có nó, với 100 lần mở nhưng mà lâu rồi thì file A nó vẫn ưu tiên hơn file B
		nhưng nhờ có decayHalfLife mà file B sẽ được ưu tiên
	*/
	boostBase int // Điểm cộng cơ bản cho mỗi lần chọn
}

type MemoryConfig struct {
	MaxFiles      int
	DecayHalfLife int64
	BoostBase     int
}

func NewFileMemory(cfg *MemoryConfig) *FileMemory {
	if cfg == nil {
		cfg = &MemoryConfig{}
	}
	if cfg.MaxFiles <= 0 {
		cfg.MaxFiles = 500
	}
	if cfg.DecayHalfLife <= 0 {
		cfg.DecayHalfLife = 43200 // 12 giờ
	}
	if cfg.BoostBase <= 0 {
		cfg.BoostBase = 5000
	}

	return &FileMemory{
		files:         make(map[string]*FileRecord),
		maxFiles:      cfg.MaxFiles,
		decayHalfLife: cfg.DecayHalfLife,
		boostBase:     cfg.BoostBase,
	}
}

/*
RecordSelection: ghi nhận hành vi user chọn một file
- query: từ khóa user đã gõ
- filePath: file user đã bấm
*/
func (fm *FileMemory) RecordSelection(query, filePath string) {
	if query == "" || filePath == "" {
		return
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()

	queryNorm := Normalize(query)
	now := time.Now().Unix()

	record, exists := fm.files[filePath]
	if !exists {
		// Kiểm tra giới hạn file trước khi thêm mới
		if len(fm.files) >= fm.maxFiles {
			fm.evictLowestScore(now)
		}
		record = &FileRecord{
			Queries: make([]string, 0, 3),
		}
		fm.files[filePath] = record
	}

	if record.SelectCount < math.MaxInt {
		record.SelectCount++
	}
	record.LastAccess = now

	// Cập nhật query list (ring buffer)
	foundQuery := false
	for _, q := range record.Queries {
		if q == queryNorm {
			foundQuery = true
			break
		}
	}
	if !foundQuery {
		if len(record.Queries) >= 3 {
			// Xóa cái cũ nhất
			record.Queries = record.Queries[1:]
		}
		record.Queries = append(record.Queries, queryNorm)
	}
}

/*
GetBoostScores: Lấy danh sách điểm cộng cho các file dựa trên query hiện tại
- Sử dụng JaroWinkler để so sánh query hiện tại với query trong lịch sử của file
*/
func (fm *FileMemory) GetBoostScores(query string) map[string]int {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	result := make(map[string]int)
	if query == "" || len(fm.files) == 0 {
		return result
	}

	queryNorm := Normalize(query)
	now := time.Now().Unix()

	for path, record := range fm.files {
		// tính độ mức độ phù hợp của query
		bestSim := 0.0
		for _, q := range record.Queries {
			sim := JaroWinkler(queryNorm, q)
			if sim > bestSim {
				bestSim = sim
			}
		}

		// ngưỡng tối thiểu để được tính là có liên quan
		if bestSim < 0.7 {
			continue
		}

		// decay = 2^(-elapsed / half-life)
		elapsed := now - record.LastAccess
		if elapsed < 0 {
			elapsed = 0
		}
		decay := math.Pow(2, -float64(elapsed)/float64(fm.decayHalfLife))

		// điểm boost tỉ lệ thuận với số lần chọn, decay theo thời gian và độ khớp query
		boost := float64(fm.boostBase) * float64(record.SelectCount) * decay * bestSim

		if boost > 0 {
			result[path] = int(boost)
		}
	}

	return result
}

/*
evictLowestScore: Xóa file có điểm frecency thấp nhất để giải phóng cache
*/
func (fm *FileMemory) evictLowestScore(now int64) {
	minScore := math.MaxFloat64
	var victim string

	for path, record := range fm.files {
		elapsed := now - record.LastAccess
		decay := math.Pow(2, -float64(elapsed)/float64(fm.decayHalfLife))
		score := float64(record.SelectCount) * decay

		if score < minScore {
			minScore = score
			victim = path
		}
	}

	if victim != "" {
		delete(fm.files, victim)
	}
}

/*
Export: Xuất dữ liệu memory ra mảng byte
- Dùng để lưu xuống disk hoặc gửi qua mạng
*/
func (fm *FileMemory) Export() ([]byte, error) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(fm.files); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

/*
Import: Nạp dữ liệu từ mảng byte vào memory
*/
func (fm *FileMemory) Import(data []byte) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	var imported map[string]*FileRecord
	if err := dec.Decode(&imported); err != nil {
		return err
	}
	fm.files = imported
	return nil
}

/*
GetRecentFiles: Lấy danh sách n file vừa được chọn gần nhất trong lịch sử
*/
func (fm *FileMemory) GetRecentFiles(limit int) []string {
	if limit <= 0 {
		return nil
	}

	fm.mu.RLock()
	defer fm.mu.RUnlock()

	if len(fm.files) == 0 {
		return nil
	}

	type fileAccess struct {
		path string
		time int64
	}
	records := make([]fileAccess, 0, len(fm.files))
	for path, r := range fm.files {
		records = append(records, fileAccess{path, r.LastAccess})
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].time > records[j].time
	})

	if limit > len(records) {
		limit = len(records)
	}

	result := make([]string, limit)
	for i := 0; i < limit; i++ {
		result[i] = records[i].path
	}
	return result
}
