package core

import (
	"container/heap"
	"runtime"
	"sync"
)

type FuzzyMatch struct {
	Index int
	Score int
}

// minHeap để lấy Top-K hiệu quả bằng partial sort O(N log K) thay vì full sort O(N log N)
type minHeap []FuzzyMatch

func (h minHeap) Len() int            { return len(h) }
func (h minHeap) Less(i, j int) bool  { return h[i].Score < h[j].Score }
func (h minHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x interface{}) { *h = append(*h, x.(FuzzyMatch)) }
func (h *minHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// Mặc định giới hạn nếu không truyền
const defaultTopK = 20

/*
FuzzyFindFiltered: Tìm kiếm fuzzy chỉ trên danh sách candidates đã lọc (Parallel)
*/
func FuzzyFindFiltered(query []byte, items [][]byte, candidates []int, baseStarts []int, limit int) []FuzzyMatch {
	if limit <= 0 {
		limit = defaultTopK
	}
	n := len(candidates)
	if n == 0 {
		return nil
	}

	// Nếu ít candidates thì chạy single-thread cho đỡ overhead
	if n < 500 {
		h := &minHeap{}
		heap.Init(h)
		for _, idx := range candidates {
			if score, matched := fuzzyScoreGreedy(query, items[idx], baseStarts[idx]); matched {
				if h.Len() < limit {
					heap.Push(h, FuzzyMatch{Index: idx, Score: score})
				} else if score > (*h)[0].Score {
					(*h)[0] = FuzzyMatch{Index: idx, Score: score}
					heap.Fix(h, 0)
				}
			}
		}
		return heapToSorted(h)
	}

	// Parallel cho lượng candidates lớn
	numCPUs := runtime.GOMAXPROCS(0)
	chunkSize := (n + numCPUs - 1) / numCPUs

	var wg sync.WaitGroup
	resultChan := make(chan []FuzzyMatch, numCPUs)

	for i := range numCPUs {
		start := i * chunkSize
		if start >= n {
			break
		}
		end := start + chunkSize
		if end > n {
			end = n
		}

		wg.Add(1)
		go func(s, e int) {
			defer wg.Done()
			h := &minHeap{}
			heap.Init(h)
			for _, idx := range candidates[s:e] {
				if score, matched := fuzzyScoreGreedy(query, items[idx], baseStarts[idx]); matched {
					if h.Len() < limit {
						heap.Push(h, FuzzyMatch{Index: idx, Score: score})
					} else if score > (*h)[0].Score {
						(*h)[0] = FuzzyMatch{Index: idx, Score: score}
						heap.Fix(h, 0)
					}
				}
			}
			resultChan <- heapToSorted(h)
		}(start, end)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	finalHeap := &minHeap{}
	heap.Init(finalHeap)
	for matches := range resultChan {
		for _, m := range matches {
			if finalHeap.Len() < limit {
				heap.Push(finalHeap, m)
			} else if m.Score > (*finalHeap)[0].Score {
				(*finalHeap)[0] = m
				heap.Fix(finalHeap, 0)
			}
		}
	}

	return heapToSorted(finalHeap)
}

/*
FuzzyFindParallel: Tìm kiếm fuzzy trên toàn bộ danh sách file (Parallel)
*/
func FuzzyFindParallel(query []byte, items [][]byte, baseStarts []int, limit int) []FuzzyMatch {
	if limit <= 0 {
		limit = defaultTopK
	}
	numItems := len(items)
	if numItems == 0 {
		return nil
	}

	numCPUs := runtime.GOMAXPROCS(0)
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
			h := &minHeap{}
			heap.Init(h)
			for j := s; j < e; j++ {
				if score, matched := fuzzyScoreGreedy(query, items[j], baseStarts[j]); matched {
					if h.Len() < limit {
						heap.Push(h, FuzzyMatch{Index: j, Score: score})
					} else if score > (*h)[0].Score {
						(*h)[0] = FuzzyMatch{Index: j, Score: score}
						heap.Fix(h, 0)
					}
				}
			}
			resultChan <- heapToSorted(h)
		}(start, end)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	finalHeap := &minHeap{}
	heap.Init(finalHeap)
	for matches := range resultChan {
		for _, m := range matches {
			if finalHeap.Len() < limit {
				heap.Push(finalHeap, m)
			} else if m.Score > (*finalHeap)[0].Score {
				(*finalHeap)[0] = m
				heap.Fix(finalHeap, 0)
			}
		}
	}

	return heapToSorted(finalHeap)
}

// heapToSorted: Chuyển minHeap thành slice đã sắp xếp giảm dần
func heapToSorted(h *minHeap) []FuzzyMatch {
	n := h.Len()
	result := make([]FuzzyMatch, n)
	for i := n - 1; i >= 0; i-- {
		result[i] = heap.Pop(h).(FuzzyMatch)
	}
	return result
}