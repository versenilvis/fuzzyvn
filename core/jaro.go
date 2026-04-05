package core

import (
	"sync"
)

// giới hạn độ dài chuỗi để sử dụng Pool (thường tên file/query < 128 chars)
const maxJaroLen = 128

// tái sử dụng mảng bool để tránh Heap Allocation liên tục
var jaroPool = sync.Pool{
	New: func() any {
		arr := [2][maxJaroLen]bool{}
		return &arr
	},
}

/*
JaroWinkler: Tính độ tương đồng giữa 2 chuỗi dạng []byte (0.0 - 1.0)
- Ưu tiên các chuỗi có prefix giống nhau.
*/
func JaroWinkler(a, b []byte) float64 {
	lA := len(a)
	lB := len(b)

	if lA == 0 && lB == 0 {
		return 1.0
	}
	if lA == 0 || lB == 0 {
		return 0.0
	}

	// Early Exit: Nếu byte đầu khác nhau, xác suất Jaro > 0.7 là rất thấp
	// Giúp bỏ qua nhanh các file không liên quan trong lịch sử
	if a[0] != b[0] {
		return 0.0
	}

	lenPrefix := 0
	maxPrefix := 4
	minLen := lA
	if lB < minLen {
		minLen = lB
	}
	if maxPrefix > minLen {
		maxPrefix = minLen
	}

	for i := 0; i < maxPrefix; i++ {
		if a[i] == b[i] {
			lenPrefix++
		} else {
			break
		}
	}

	similarity := jaroSim(a, b)
	// tăng điểm nếu có prefix giống nhau (chuẩn 0.1)
	return similarity + (0.1 * float64(lenPrefix) * (1.0 - similarity))
}

func jaroSim(str1, str2 []byte) float64 {
	l1 := len(str1)
	l2 := len(str2)

	// ngưỡng tìm kiếm ký tự khớp
	matchDistance := l1
	if l2 > matchDistance {
		matchDistance = l2
	}
	matchDistance = matchDistance/2 - 1
	if matchDistance < 0 {
		matchDistance = 0
	}

	var str1Matches, str2Matches []bool

	// FAST PATH: Sử dụng Buffer từ Pool nếu chuỗi đủ ngắn
	if l1 <= maxJaroLen && l2 <= maxJaroLen {
		buf := jaroPool.Get().(*[2][maxJaroLen]bool)
		defer jaroPool.Put(buf)
		
		// Reset vùng nhớ cần dùng
		for i := 0; i < l1; i++ { buf[0][i] = false }
		for i := 0; i < l2; i++ { buf[1][i] = false }
		
		str1Matches = buf[0][:l1]
		str2Matches = buf[1][:l2]
	} else {
		// FALLBACK: Cấp phát động nếu chuỗi quá dài
		str1Matches = make([]bool, l1)
		str2Matches = make([]bool, l2)
	}

	matches := 0.0
	for i := 0; i < l1; i++ {
		start := i - matchDistance
		if start < 0 {
			start = 0
		}
		end := i + matchDistance + 1
		if end > l2 {
			end = l2
		}

		for k := start; k < end; k++ {
			if str2Matches[k] || str1[i] != str2[k] {
				continue
			}
			str1Matches[i] = true
			str2Matches[k] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	transpositions := 0.0
	k := 0
	for i := 0; i < l1; i++ {
		if !str1Matches[i] {
			continue
		}
		for !str2Matches[k] {
			k++
		}
		if str1[i] != str2[k] {
			transpositions += 1.0
		}
		k++
	}

	transpositions /= 2
	
	invM := 1.0 / matches
	return (matches/float64(l1) + matches/float64(l2) + (matches-transpositions)*invM) / 3.0
}
