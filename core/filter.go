/*
trước khi search, để tiết kiệm thời gian ta phải lọc ra các file có khả năng chứa các
từ khoá là file ta cần tìm trước
*/

package core

import (
	"math/bits"
	"runtime"
	"sync"
)

/*
Unigram Filter (K-of-N Match)
- Vứt bỏ ký tự xuất hiện ở > 85% lượng file (ví dụ "a", "e", ".", "/")
- Khi có file bị xoá, thay vì build lại ta đánh dấu bia mộ lên file đó,
khi search, nếu file trong thùng râc (bin) thì bỏ qua
*/

type UnigramFilter struct {
	// 256 tương ứng 256 ký tự ASCII
	// mỗi bit 0 và 1 đại diện cho 1 file
	// với 1 con số uint64 có 64 bit, nó sẽ chứa được 64 file, nên là dùng 1 slice để quản lý mỗi số sẽ chứa 64 file
	// hãy hiểu ta có 256 dòng, mỗi dòng có chứa tất cả các cột tương ứng với file
	// Ví dụ: "A" -> File 1 và File 63 có -> tương ứng bit 1 và bit 63 bật lên 1, còn lại là 0
	Bitsets    [256][]uint64
	Bin        []uint64 // Chứa cờ bit 1 cho các file đã bị xóa
	NumTargets int      // bao nhiêu file cần tìm
}

// NewUnigramFilter: Xây dựng Inverted Bitsets 1 lần duy nhất lúc nạp danh sách file
// LƯU Ý: nó build index khi server chạy chứ không phải lúc đang search
func NewUnigramFilter(targets [][]byte) *UnigramFilter {
	numTargets := len(targets)
	// làm tròn lên
	blocks := (numTargets + 63) / 64

	uf := &UnigramFilter{
		NumTargets: numTargets,
		Bin:        make([]uint64, blocks),
	}

	// Cấp phát trước bộ nhớ cho 256 mảng bit
	for i := range uf.Bitsets {
		uf.Bitsets[i] = make([]uint64, blocks)
	}

	// đánh dấu các mảng bit
	numWorkers := runtime.GOMAXPROCS(0)
	if numWorkers > 16 {
		numWorkers = 16 // tránh tốn chi phí của Goroutine nếu > 16 core
	}
	// chia workload theo khối 64
	blocksPerWorker := (blocks + numWorkers - 1) / numWorkers
	if blocksPerWorker == 0 {
		blocksPerWorker = 1
	}

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		startBlock := i * blocksPerWorker
		endBlock := startBlock + blocksPerWorker
		if startBlock >= blocks {
			break
		}
		if endBlock > blocks {
			endBlock = blocks
		}

		startDoc := startBlock * 64
		endDoc := endBlock * 64
		if endDoc > numTargets {
			endDoc = numTargets
		}

		wg.Add(1)

		go func(start, end int) {
			defer wg.Done()
			for docID := start; docID < end; docID++ {
				target := targets[docID]
				blockIdx := docID / 64
				// chia lấy dư 64 để biết nó có giá trị bao nhiêu trước, ví dụ 130/64 = 2, nó ở vị trí 2 block 2
				// ban đầu mọi file đều mang bit 0
				// dịch trái 1 bit để đánh dấu thành 0001 chẳng hạn, để bật 1 bit file đó lên
				bitPos := uint64(1) << (docID % 64)

				// target là path hoặc tên file, chạy qua từng chữ cái 1
				for _, b := range target {
					// với mỗi chữ cái của tên file, nó sẽ bật bit tương ứng với file đó lên
					uf.Bitsets[b][blockIdx] |= bitPos
				}
			}
		}(startDoc, endDoc)
	}
	wg.Wait()

	threshold85 := (numTargets * 85) / 100
	for i := range uf.Bitsets {
		popcount := 0
		for _, block := range uf.Bitsets[i] {
			popcount += bits.OnesCount64(block)
		}

		// nếu ký tự này có mặt ở >= 85% tổng số file -> vô dụng
		if popcount >= threshold85 {
			// giải phóng RAM trả lại OS + bỏ ký tự này
			uf.Bitsets[i] = nil
		}
	}

	return uf
}

// DeleteFile: thay vì build lại, chỉ đánh dấu vào bin trong quá trình sử dụng
func (uf *UnigramFilter) DeleteFile(docID int) {
	if docID >= uf.NumTargets {
		return
	}
	blockIdx := docID / 64
	bitPos := uint64(1) << (docID % 64)
	// đánh dấu bỏ vào bin
	uf.Bin[blockIdx] |= bitPos
}

// Filter trả về ID các file đủ tiêu chuẩn qua bài test K/N
// Phải qua hàm filter trước, không có thì gọi Searcher
func (uf *UnigramFilter) Filter(pattern []byte) []int {
	uniqueChars := make([]byte, 0, len(pattern))
	var seen [4]uint64
	for _, b := range pattern {
		idx := b / 64
		bit := uint64(1) << (b % 64)
		if seen[idx]&bit == 0 {
			seen[idx] |= bit
			// chỉ lấy vào uniqueChars những chữ có ích
			if uf.Bitsets[b] != nil {
				uniqueChars = append(uniqueChars, b)
			}
		}
	}
	// code trên nhằm mục đích remove duplicate

	n := len(uniqueChars)
	// nếu dưới 2 thì bỏ qua để gọi Searcher
	if n < 2 {
		return nil
	}

	// Tính typo tolerance
	// 2-3 char: bắt buộc tìm đủ (k = n)
	// 4-5 char: cho phép sai lệch / missing 1 chữ (k = n - 1)
	// >=6 char: cho phép gõ sai hẳn 2 chữ (k = n - 2)
	threshold := n
	if n >= 6 {
		threshold = n - 2
	} else if n >= 4 {
		threshold = n - 1
	}

	// khởi tạo hit count
	counts := make([]uint8, uf.NumTargets)

	// cộng điểm cho các file thoả mãn tìm thấy ký tự
	for _, b := range uniqueChars {
		set := uf.Bitsets[b]
		for blockIdx, block := range set {
			for block != 0 {
				trailing := bits.TrailingZeros64(block) // tìm xem bit 1 đang ở vị trí nào để lấy docID
				docID := blockIdx*64 + trailing

				counts[docID]++

				// lấy được docID rồi ta phải xoá bit 1 đó đi,
				// nếu không xoá thì ở loop tiếp theo nó sẽ lại lấy cái docID nữa, dẫn tới loop vô hạn
				block &= (block - 1) // xoá bit ngoài cùng bên phải, biến bit đó thành 0
			}
		}
	}

	// filter ra
	validIDs := make([]int, 0, 1000)
	thresh8 := uint8(threshold)
	for docID, count := range counts {
		if count >= thresh8 {
			// chia 64 để biết nó nằm ở block nào
			blockIdx := docID / 64
			// sau khi biết block nào
			// chia lấy dư 64 để biết nó ở vị trí nào, ví dụ 130%64 = 2, nó ở vị trí 2 block 2
			// ban đầu mọi file đều mang bit 0
			// dịch trái 1 bit để đánh dấu thành 0001 chẳng hạn, để bật 1 bit file đó lên
			bitPos := uint64(1) << (docID % 64)
			// Nếu AND bit ra 0 nghĩa là file này chưa hề bị đánh dấu Xóa
			if uf.Bin[blockIdx]&bitPos == 0 {
				validIDs = append(validIDs, docID)
			}
		}
	}

	return validIDs
}
