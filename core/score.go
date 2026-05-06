package core

/*
fuzzyScoreGreedy: Tính điểm fuzzy match sử dụng thuật toán tham lam
- pattern: Query đã normalize
- target: Target string đã normalize
- baseStart: Vị trí bắt đầu của filename trong target
*/
func FuzzyScoreGreedy(pattern []byte, target []byte, baseStart int) (int, bool) {
	lenP := len(pattern)
	lenT := len(target)

	if lenP > lenT {
		return 0, false
	}

	totalScore := 0
	patternIdx := 0
	firstMatchIdx := -1
	lastMatchIdx := -1
	lastMatchedTargetIdx := -2

	// Duyệt 1 lần duy nhất qua target
	for i := 0; i < lenT; i++ {
		if patternIdx < lenP && target[i] == pattern[patternIdx] {
			if firstMatchIdx == -1 {
				firstMatchIdx = i
			}
			lastMatchIdx = i

			// Bonus điểm cực lớn nếu khớp liên tiếp (contiguous match)
			if i == lastMatchedTargetIdx+1 {
				totalScore += 200
			}

			// Bonus điểm nếu khớp ký tự đầu từ (word boundary)
			if i == 0 {
				totalScore += 80
			} else {
				switch target[i-1] {
				case '/', '\\', '_', '-', '.', ' ':
					totalScore += 80
				default:
					totalScore += 10
				}
			}

			// Bonus nếu khớp trong phần tên file
			if i < baseStart {
				totalScore += 15
			}

			lastMatchedTargetIdx = i
			patternIdx++
		}
	}

	// Nếu không khớp hết Query -> Loại
	if patternIdx != lenP {
		return 0, false
	}

	// Tính điểm cơ bản
	// Match càng gọn (khoảng cách đầu-cuối ngắn) thì điểm càng cao
	matchRange := lastMatchIdx - firstMatchIdx + 1
	if matchRange < lenP {
		matchRange = lenP
	}
	baseScore := (lenP * 100) - (matchRange-lenP)*5
	// TỪ CHỐI "ĐỘ LOÃNG":
	// Nếu baseScore <= 0, nghĩa là các ký tự match nằm cách nhau quá xa (độ loãng quá cao).
	// Ta reject luôn để tránh lệnh rác khổng lồ hứng mọi từ khóa.
	if baseScore <= 0 {
		return 0, false
	}
	totalScore += baseScore

	// Phạt độ dài: Lệnh càng dài (chứa nhiều rác) càng bị trừ điểm
	lengthPenalty := lenT * 2
	totalScore -= lengthPenalty

	// Tier 1: Query là prefix chính xác của filename
	if baseStart >= lenP {
		isPerfectStart := true
		for i := 0; i < lenP; i++ {
			if target[i] != pattern[i] {
				isPerfectStart = false
				break
			}
		}
		if isPerfectStart {
			totalScore += 1000000
			return totalScore, true
		}
	}

	// Tier 2: Filename ngắn chứa tất cả ký tự của query
	// Check điều kiện rẻ nhất trước để tránh tính charBucket cho hàng vạn file
	if baseStart <= lenP*3 {
		var charBucket [256]int8
		for i := 0; i < baseStart; i++ {
			charBucket[target[i]]++
		}
		filenameHits := 0
		for _, b := range pattern {
			if charBucket[b] > 0 {
				charBucket[b]--
				filenameHits++
			}
		}
		if filenameHits == lenP {
			totalScore += 500000
			return totalScore, true
		}
	}

	// Tier 3: Có ít nhất 1 match trong filename
	if firstMatchIdx < baseStart {
		totalScore += (totalScore * 200) / 100
	} else {
		// Tier 4: Chỉ match ở phần path -> phạt nặng
		totalScore -= (lenT / 3)
	}

	return totalScore, true
}
