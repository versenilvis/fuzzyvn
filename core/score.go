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

	// file name và dir
	baseStart := 0
	for i := lenT - 1; i >= 0; i-- {
		if target[i] == '/' || target[i] == '\\' {
			baseStart = i + 1
			break
		}
	}

	// Index của ký tự khớp trước đó trong target
	prevMatchIdx := -1

	// Index hiện tại đang xét trong target
	targetIdx := 0

	lastMatchIdx := -1

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
						// Tính năng này có thể hơi mờ nhạt nếu target đã bị ToLower() ở vòng ngoài,
						// nhưng giữ lại để phòng hờ trường hợp dùng mảng nguyên bản.
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
					break // Tìm thấy WordStart -> Chốt lệnh 
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

		totalScore += bestScore
		prevMatchIdx = bestIdx
		lastMatchIdx = bestIdx
		targetIdx = bestIdx + 1
	}

	// Penalty khoảng cách tổng thể nếu độ dài pattern quá ngắn so với target
	lenDiff := lenT - lenP
	if lenDiff > 0 {
		totalScore -= lenDiff * 2
	}

	// Thưởng điểm dựa vào file base
	// Tách luồng điểm thư mục và tên file để ưu tiên những kết quả tiệm cận với tên file
	
	isFilenameMatch := false
	if lastMatchIdx >= baseStart {
		isFilenameMatch = true
	}

	baseName := target[baseStart:]
	isExactFilename := false
	
	if len(baseName) == lenP {
		isExactFilename = true
		for i := range baseName {
			if baseName[i] != pattern[i] {
				isExactFilename = false
				break
			}
		}
	}

	if isExactFilename {
		// bonus 40% điểm nếu match chính xác cái tên file 
		// ví dụ: gõ "user.go" tìm ra đúng "/src/models/user.go"
		totalScore += (totalScore * 40) / 100
	} else if isFilenameMatch {
		// bonus 16% điểm nếu ký tự match cuối cùng nằm bên trong phần tên file
		// ví dụ gõ "usgo" na ná với "user.go" -> ưu tiên hơn so với nằm ở thư mục "usgo/main.ts"
		totalScore += (totalScore * 16) / 100
	} else {
		// nếu kết quả rơi vào phần dir, kiểm tra xem có phải là file đặc biệt thường dùng không
		strBase := string(baseName)
		if isEntryPoint(strBase) {
			// bonus 5% cho các entry point
			totalScore += (totalScore * 5) / 100
		}
	}

	return totalScore, true
}