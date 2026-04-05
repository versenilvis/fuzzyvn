package core

import (
	"runtime"
	"sort"
	"sync"
)

type FuzzyMatch struct {
	Index int
	Score int
}

/*
FuzzyFindFiltered: Tìm kiếm fuzzy chỉ trên danh sách các candidates đã được lọc
- query: Query đã normalize
- items: Toàn bộ danh sách file đã normalize
- candidates: Danh sách index các file tiềm năng (từ UnigramFilter)
- baseStarts: Mảng pre-computed vị trí bắt đầu của filename
*/
func FuzzyFindFiltered(query []byte, items [][]byte, candidates []int, baseStarts []int) []FuzzyMatch {
	if len(candidates) == 0 {
		return nil
	}

	results := make([]FuzzyMatch, 0, len(candidates))
	for _, idx := range candidates {
		if score, matched := fuzzyScoreGreedy(query, items[idx], baseStarts[idx]); matched {
			results = append(results, FuzzyMatch{
				Index: idx,
				Score: score,
			})
		}
	}

	// Sắp xếp theo điểm số giảm dần
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

/*
FuzzyFindParallel: Tìm kiếm fuzzy trên toàn bộ danh sách file (Parallel)
- Dùng làm phương án dự phòng khi UnigramFilter không lọc được gì
*/
func FuzzyFindParallel(query []byte, items [][]byte, baseStarts []int) []FuzzyMatch {
	numItems := len(items)
	if numItems == 0 {
		return nil
	}

	numCPUs := runtime.NumCPU()
	chunkSize := (numItems + numCPUs - 1) / numCPUs

	var wg sync.WaitGroup
	resultChan := make(chan []FuzzyMatch, numCPUs)

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
		go func(s, e int) {
			defer wg.Done()
			var chunkMatches []FuzzyMatch
			for j, item := range items[s:e] {
				idx := s + j
				if score, matched := fuzzyScoreGreedy(query, item, baseStarts[idx]); matched {
					chunkMatches = append(chunkMatches, FuzzyMatch{
						Index: idx,
						Score: score,
					})
				}
			}
			resultChan <- chunkMatches
		}(start, end)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var allMatches []FuzzyMatch
	for matches := range resultChan {
		allMatches = append(allMatches, matches...)
	}

	sort.Slice(allMatches, func(i, j int) bool {
		return allMatches[i].Score > allMatches[j].Score
	})

	return allMatches
}