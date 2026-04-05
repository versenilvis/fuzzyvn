package core

import (
	"unicode"
)

/*
isEntryPoint: Kiểm tra xem filename có phải là file khởi đầu quan trọng không
*/
func isEntryPoint(filename string) bool {
	switch filename {
	case "mod.rs", "lib.rs", "main.rs", // Rust
		"index.js", "index.jsx", "index.ts", "index.tsx", "index.mjs", "index.cjs", // JS/TS
		"index.vue", "App.vue", // Vue
		"index.html",                            // Web
		"__init__.py", "__main__.py", "main.py", // Python
		"main.go",                      // Go
		"main.c", "main.cpp", "main.h", // C/C++
		"index.php",           // PHP
		"main.rb", "index.rb", // Ruby
		"Main.java", "Application.java", // Java
		"main.swift", // Swift
		"main.dart":  // Dart/Flutter
		return true
	}
	return false
}

/*
fuzzyScoreGreedy: Tính điểm fuzzy match sử dụng thuật toán tham lam
- pattern: Query đã normalize
- target: Target string đã normalize
- baseStart: Vị trí bắt đầu của filename trong target
*/
func fuzzyScoreGreedy(pattern []byte, target []byte, baseStart int) (int, bool) {
	lenP := len(pattern)
	lenT := len(target)

	if lenP > lenT {
		return 0, false
	}

	totalScore := 0
	patternIdx := 0
	firstMatchIdx := -1
	lastMatchIdx := -1

	// Duyệt 1 lần duy nhất qua target
	for i, char := range target {
		if patternIdx < lenP && char == pattern[patternIdx] {
			if firstMatchIdx == -1 {
				firstMatchIdx = i
			}
			lastMatchIdx = i

			// Bonus điểm nếu khớp ký tự đầu từ
			isWordStart := false
			if i == 0 {
				isWordStart = true
			} else {
				prev := target[i-1]
				if prev == '/' || prev == '\\' || prev == '_' || prev == '-' || prev == '.' || prev == ' ' {
					isWordStart = true
				} else if unicode.IsLower(rune(prev)) && unicode.IsUpper(rune(target[i])) {
					isWordStart = true
				}
			}

			if isWordStart {
				totalScore += 80
			} else {
				totalScore += 10
			}

			// Bonus nếu khớp trong phần tên file
			if i < baseStart {
				totalScore += 15
			}

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
	if baseScore < 0 {
		baseScore = 0
	}
	totalScore += baseScore

	// Khớp chính xác tên file -> Điểm cao
	// filename nằm ở [0, baseStart) trong target
	if baseStart == lenP {
		isExactBase := true
		for i, char := range target[:lenP] {
			if char != pattern[i] {
				isExactBase = false
				break
			}
		}
		if isExactBase {
			// Bonus 40% nếu khớp chính xác tên file
			totalScore += (totalScore * 40) / 100
		}
	} else if firstMatchIdx < baseStart {
		// Bonus 16% nếu có ít nhất 1 match trong phần filename
		totalScore += (totalScore * 16) / 100
	} else {
		// Chỉ match ở phần path -> check entry point
		filename := string(target[:baseStart])
		if isEntryPoint(filename) {
			totalScore += (totalScore * 5) / 100
		}
	}

	return totalScore, true
}
