package core

import (
	"bytes"
	"runtime"
	"slices"
	"sync"
)

type FuzzyMatch struct {
	Index int
	Score int
	Exact bool
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
func FuzzyFind(pattern []byte, targets [][]byte) []FuzzyMatch {
	if len(pattern) == 0 {
		return nil
	}
	// Pre-allocate slice kết quả để tránh resize liên tục
	results := make([]FuzzyMatch, 0, 1000)

	for idx := range targets {
		score, matched := fuzzyScoreGreedy(pattern, targets[idx])

		if matched {
			exact := bytes.Contains(targets[idx], pattern)
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
	slices.SortFunc(results, func(a, b FuzzyMatch) int {
		return b.Score - a.Score
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
func FuzzyFindParallel(pattern []byte, targets [][]byte) []FuzzyMatch {
	if len(pattern) == 0 {
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
				score, matched := fuzzyScoreGreedy(pattern, targets[i])

				if matched {
					exact := bytes.Contains(targets[i], pattern)
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
	slices.SortFunc(allResults, func(a, b FuzzyMatch) int {
		return b.Score - a.Score
	})

	return allResults
}