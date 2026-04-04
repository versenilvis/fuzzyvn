package core

import (
	"unicode"
)

/*
fuzzyScoreGreedy: Tính điểm fuzzy match sử dụng thuật toán tham lam
- pattern: Query đã normalize
- target: Target string đã normalize
- Trả về: (matched bool, score int, positions []int)
- Cách này có 1 vấn đề nho nhỏ, là do tham lam
- Nó có thể bỏ qua một match tốt hơn ở sau để chọn match đầu tiên tìm được
- Nhưng bù lại cực nhanh vì chỉ duyệt target 1 lần
*/
func fuzzyScoreGreedy(pattern []byte, target []byte) (int, bool) {
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

	// Bounds Check Elimination (BCE) hints:
	// Giúp Go compiler bỏ qua thao tác kiểm tra an toàn tràn mảng (bounds check)
	_ = target[lenT-1]
	_ = pattern[lenP-1]

	for pIdx := 0; pIdx < lenP; pIdx++ {
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
					if isSeparator(rune(prevChar)) {
						isWordStart = true
					} else if unicode.IsLower(rune(prevChar)) && unicode.IsUpper(rune(tChar)) {
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