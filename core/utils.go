package core

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func isSeparator(r rune) bool {
	// Liệt kê các ký tự ngăn cách phổ biến trong code/path
	return r == '/' || r == '\\' || r == '_' || r == '-' || r == '.' || r == ' ' || r == ':'
}

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

func CountWordMatches(queryWords []string, target string) int {
	if len(target) < 2 {
		return 0
	}
	targetWords := strings.FieldsFunc(target, isSeparator)
	if len(targetWords) == 0 {
		return 0
	}
	count := 0
	for _, qWord := range queryWords {
		if len(qWord) < 2 {
			continue
		}
		for _, tWord := range targetWords {
			if len(tWord) < 2 {
				continue
			}
			// Exact match
			if qWord == tWord {
				count++
				break
			}
			// Fuzzy match: cho phép 1 lỗi nếu từ >= 3 ký tự
			// CHỈ check Levenshtein nếu độ dài gần bằng nhau
			if len(qWord) >= 3 && len(tWord) >= 3 && abs(len(qWord)-len(tWord)) <= 1 {
				dist := LevenshteinRatio(qWord, tWord)
				if dist <= 1 {
					count++
					break
				}
			}
		}
	}
	return count
}

func Normalize(s string) string {
	// Nếu toàn là ASCII (Tiếng Anh, Code) -> Lowercase và trả về ngay
	// Đây là trường hợp phổ biến nhất
	isASCII := true
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			isASCII = false
			break
		}
	}
	if isASCII {
		return strings.ToLower(s)
	}

	// MacOS dùng NFD nên cần check
	if !norm.NFC.IsNormalString(s) {
		s = norm.NFC.String(s)
	}

	// Duyệt trực tiếp trên byte, không decode rune
	// Ký tự tiếng Việt NFC nằm trong phạm vi 2-3 byte UTF-8:
	//   2-byte: 0xC0-0xDF + 1 byte tiếp theo (á, à, â, đ, é, ê, ...)
	//   3-byte: 0xE0-0xEF + 2 byte tiếp theo (ắ, ằ, ẳ, ớ, ờ, ợ, ...)
	buf := make([]byte, 0, len(s))
	src := []byte(s)
	i := 0

	for i < len(src) {
		b := src[i]

		// xử lý trực tiếp không cần decode
		if b < 128 {
			if b >= 'A' && b <= 'Z' {
				buf = append(buf, b+32) // toLower
			} else {
				buf = append(buf, b)
			}
			i++
			continue
		}

		// 2-byte UTF-8 sequence: 0xC0-0xDF
		if b >= 0xC0 && b <= 0xDF && i+1 < len(src) {
			b2 := src[i+1]
			mapped := mapViet2Byte(b, b2)
			if mapped != 0 {
				buf = append(buf, mapped)
				i += 2
				continue
			}
			// không phải tiếng Việt -> giữ nguyên 2 byte (lowercase bằng unicode)
			r, size := utf8.DecodeRune(src[i:])
			r = unicode.ToLower(r)
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				var tmp [4]byte
				n := utf8.EncodeRune(tmp[:], r)
				buf = append(buf, tmp[:n]...)
			}
			i += size
			continue
		}

		// 3-byte UTF-8 sequence: 0xE0-0xEF
		if b >= 0xE0 && b <= 0xEF && i+2 < len(src) {
			b2 := src[i+1]
			b3 := src[i+2]
			mapped := mapViet3Byte(b, b2, b3)
			if mapped != 0 {
				buf = append(buf, mapped)
				i += 3
				continue
			}
			// không phải tiếng Việt -> giữ nguyên
			r, size := utf8.DecodeRune(src[i:])
			r = unicode.ToLower(r)
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				var tmp [4]byte
				n := utf8.EncodeRune(tmp[:], r)
				buf = append(buf, tmp[:n]...)
			}
			i += size
			continue
		}

		// 4-byte hoặc invalid -> decode bình thường
		r, size := utf8.DecodeRune(src[i:])
		r = unicode.ToLower(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			var tmp [4]byte
			n := utf8.EncodeRune(tmp[:], r)
			buf = append(buf, tmp[:n]...)
		}
		i += size
	}

	return string(buf)
}

// mapViet2Byte: map 2-byte UTF-8 Vietnamese chars -> ASCII
// trả về 0 nếu không phải ký tự Việt cần map
func mapViet2Byte(b1, b2 byte) byte {
	switch b1 {
	case 0xC3:
		// lowercase: à=0xA0, á=0xA1, â=0xA2, ã=0xA3
		// è=0xA8, é=0xA9, ê=0xAA
		// ì=0xAC, í=0xAD
		// ò=0xB2, ó=0xB3, ô=0xB4, õ=0xB5
		// ù=0xB9, ú=0xBA
		// ý=0xBD
		// uppercase: À=0x80, Á=0x81, Â=0x82, Ã=0x83
		// È=0x88, É=0x89, Ê=0x8A
		// Ì=0x8C, Í=0x8D
		// Ò=0x92, Ó=0x93, Ô=0x94, Õ=0x95
		// Ù=0x99, Ú=0x9A
		// Ý=0x9D
		switch b2 {
		case 0x80, 0x81, 0x82, 0x83, 0xA0, 0xA1, 0xA2, 0xA3:
			return 'a'
		case 0x88, 0x89, 0x8A, 0xA8, 0xA9, 0xAA:
			return 'e'
		case 0x8C, 0x8D, 0xAC, 0xAD:
			return 'i'
		case 0x92, 0x93, 0x94, 0x95, 0xB2, 0xB3, 0xB4, 0xB5:
			return 'o'
		case 0x99, 0x9A, 0xB9, 0xBA:
			return 'u'
		case 0x9D, 0xBD:
			return 'y'
		}
	case 0xC4:
		// Đ=0x90, đ=0x91
		if b2 == 0x90 || b2 == 0x91 {
			return 'd'
		}
		// Ă=0x82, ă=0x83
		if b2 == 0x82 || b2 == 0x83 {
			return 'a'
		}
	case 0xC6:
		// Ơ=0xA0, ơ=0xA1
		if b2 == 0xA0 || b2 == 0xA1 {
			return 'o'
		}
		// Ư=0xAF, ư=0xB0
		if b2 == 0xAF || b2 == 0xB0 {
			return 'u'
		}
	}
	return 0
}

// mapViet3Byte: map 3-byte UTF-8 Vietnamese chars -> ASCII
// bao gồm các ký tự có dấu nặng: ắ, ằ, ẳ, ẵ, ặ, ấ, ầ, ẩ, ẫ, ậ ...
func mapViet3Byte(b1, b2, b3 byte) byte {
	if b1 != 0xE1 {
		return 0
	}

	switch b2 {
	case 0xBA:
		// 0xE1 0xBA 0x80-0xBF
		// ả=A2/A3, ạ=A0/A1, ấ=A4/A5, ầ=A6/A7, ẩ=A8/A9, ẫ=AA/AB, ậ=AC/AD
		// ắ=AE/AF, ằ=B0/B1, ẳ=B2/B3, ẵ=B4/B5, ặ=B6/B7
		// ẹ=B8/B9, ẻ=BA/BB, ẽ=BC/BD, ế=BE/BF
		if b3 >= 0xA0 && b3 <= 0xB7 {
			return 'a'
		}
		if b3 >= 0xB8 && b3 <= 0xBF {
			return 'e'
		}
		// Ả=0x82, ả=0x83, Ạ=0x80, ạ=0x81...
		if b3 >= 0x80 && b3 <= 0x9F {
			// 80-87 = a variants, 88-8D = e variants, 8E-93 = i? check
			// thực tế Unicode block 1EA0-1EBF:
			// 1EA0=Ạ, 1EA1=ạ, 1EA2=Ả, 1EA3=ả, 1EA4=Ấ, 1EA5=ấ, 1EA6=Ầ, 1EA7=ầ
			// 1EA8=Ẩ, 1EA9=ẩ, 1EAA=Ẫ, 1EAB=ẫ, 1EAC=Ậ, 1EAD=ậ, 1EAE=Ắ, 1EAF=ắ
			// 1EB0=Ằ, 1EB1=ằ, 1EB2=Ẳ, 1EB3=ẳ, 1EB4=Ẵ, 1EB5=ẵ, 1EB6=Ặ, 1EB7=ặ
			// 1EB8=Ẹ, 1EB9=ẹ, 1EBA=Ẻ, 1EBB=ẻ, 1EBC=Ẽ, 1EBD=ẽ, 1EBE=Ế, 1EBF=ế
			// UTF-8 encoding: 0xE1 0xBA 0x80-0x9F maps to U+1E80-1E9F
			// nhưng U+1E80-1E9F là Ẁ, ẁ, Ẃ, ẃ (Welsh W variants) không phải tiếng Việt
			return 0
		}
	case 0xBB:
		// 0xE1 0xBB 0x80-0xBF
		// Unicode U+1EC0-1EFF
		// 1EC0=Ề, 1EC1=ề, 1EC2=Ể, 1EC3=ể, 1EC4=Ễ, 1EC5=ễ, 1EC6=Ệ, 1EC7=ệ -> e
		// 1EC8=Ỉ, 1EC9=ỉ, 1ECA=Ị, 1ECB=ị -> i
		// 1ECC=Ọ, 1ECD=ọ, 1ECE=Ỏ, 1ECF=ỏ, 1ED0=Ố, 1ED1=ố, 1ED2=Ồ, 1ED3=ồ -> o
		// 1ED4=Ổ, 1ED5=ổ, 1ED6=Ỗ, 1ED7=ỗ, 1ED8=Ộ, 1ED9=ộ -> o
		// 1EDA=Ớ, 1EDB=ớ, 1EDC=Ờ, 1EDD=ờ, 1EDE=Ở, 1EDF=ở, 1EE0=Ỡ, 1EE1=ỡ, 1EE2=Ợ, 1EE3=ợ -> o
		// 1EE4=Ụ, 1EE5=ụ, 1EE6=Ủ, 1EE7=ủ, 1EE8=Ứ, 1EE9=ứ -> u
		// 1EEA=Ừ, 1EEB=ừ, 1EEC=Ử, 1EED=ử, 1EEE=Ữ, 1EEF=ữ, 1EF0=Ự, 1EF1=ự -> u
		// 1EF2=Ỳ, 1EF3=ỳ, 1EF4=Ỵ, 1EF5=ỵ, 1EF6=Ỷ, 1EF7=ỷ, 1EF8=Ỹ, 1EF9=ỹ -> y
		// UTF-8: b3 = codepoint & 0x3F | 0x80
		// U+1EC0 -> 0xE1 0xBB 0x80, U+1EC7 -> 0xE1 0xBB 0x87
		// e: 0x80-0x87
		if b3 >= 0x80 && b3 <= 0x87 {
			return 'e'
		}
		// i: 0x88-0x8B
		if b3 >= 0x88 && b3 <= 0x8B {
			return 'i'
		}
		// o: 0x8C-0xA3
		if b3 >= 0x8C && b3 <= 0xA3 {
			return 'o'
		}
		// u: 0xA4-0xB1
		if b3 >= 0xA4 && b3 <= 0xB1 {
			return 'u'
		}
		// y: 0xB2-0xB9
		if b3 >= 0xB2 && b3 <= 0xB9 {
			return 'y'
		}
	}
	return 0
}

func FastSubstring(s string, n int) string {
	if len(s) <= n {
		return s
	}

	count := 0
	/*
		Tại sao ta không dùng for i := 0; i < len(s); i++?
		Hàm len(s) trả về số lượng BYTE, không phải số lượng KÝ TỰ
		Ta có thể lợi dụng cách range hoạt động trong Go
		range s trong Go sẽ tự động nhảy theo TỪNG KÝ TỰ (Rune), không phải từng byte
	*/
	for i := range s {
		if count == n {
			// Tại thời điểm này, 'i' đang đứng đúng ở vị trí byte kết thúc ký tự thứ n
			// s[:i] là thao tác "Slice string"
			// Nó KHÔNG copy dữ liệu, nó chỉ trỏ vào vùng nhớ cũ
			return s[:i] // Cắt chuỗi tại byte index (Zero Alloc)
		}
		count++
	}
	/*
		Ví dụ:
		Chuỗi "âb" (giả sử â là 2 byte, b là 1 byte). Tổng 3 byte. Cần lấy 1 ký tự (n=1)
		Vòng lặp chạy lần 1: Đọc chữ â
		count tăng lên 1. count == n (1==1)
		Biến i lúc này đang ở vị trí byte tiếp theo (tức là byte số 2).
		return s[:2] -> Trả về đúng chữ â
	*/

	return s
}

// func containsRunes(target []rune, pattern []rune) bool {
// 	if len(pattern) == 0 {
// 		return true
// 	}
// 	if len(pattern) > len(target) {
// 		return false
// 	}

// 	p0 := pattern[0]
// 	n := len(pattern)
// 	for i := 0; i <= len(target)-n; i++ {
// 		if target[i] == p0 && slices.Equal(target[i:i+n], pattern) {
// 			return true
// 		}
// 	}
// 	return false
// }

/*
- Levenshtein Distance: https://viblo.asia/p/khoang-cach-levenshtein-va-fuzzy-query-trong-elasticsearch-jvElaOXAKkw
- Bạn hiểu nôm na là để tính độ sai lệch khi gõ sai, tìm kết quả gần khớp với ý muốn của bạn nhất
- Mính sẽ chỉ triển khai cái nào cần cho tiếng Việt thôi, Trung, Hàn, Nhật,... bỏ qua
- Mục tiêu là biến chuỗi s1 thành s2
- Tại mỗi bước so sánh ký tự, ta có 3 quyền lựa chọn, ta sẽ chọn cái nào tốn ít chi phí nhất (minVal):
+ Xóa bỏ ký tự ở s1 (Chi phí +1)
+ Thêm ký tự vào s1 để giống s2 (Chi phí +1)
+ Thay thế:
> Nếu 2 ký tự giống nhau: Không mất phí (+0)
> Nếu khác nhau: Thay ký tự này bằng ký tự kia (+1)
NOTE: 1 điều lưu ý là ta không cần quan tâm chữ hoa, chữ thường vì đã chuẩn hóa rồi
*/

func LevenshteinRatio(s1, s2 string) int {
	/*
		Đây là trường hợp biến chuỗi s1 thành "chuỗi rỗng"
		Ví dụ s1 = "ABC", s2 = ""
		Biến "" thành "" mất 0 bước (column[0] = 0)
		Biến "A" thành "" mất 1 bước xóa (column[1] = 1)
		Biến "AB" thành "" mất 2 bước xóa (column[2] = 2)
		Lúc này mảng column trông như thế này: [0, 1, 2, 3, ... len(s1)]
		Tương ứng tăng dần từ 0 đến len(s1) là chi phí biến thành chuỗi rỗng
	*/
	s1Len := len(s1)
	s2Len := len(s2)

	/*
		Cái này hiểu đơn giản là
		Ví dụ như: "" -> "ABC" thì ta lấy luôn độ dài s2
		Ngược lại "ABC" -> "" thì lấy độ dài s1
		Vì rõ ràng số bước thay đổi từ rỗng thành text, hay text thành rỗng tốn số bước
		đúng bằng độ dài của nó
		Điều này giúp ta bỏ qua mấy bước bên dưới, làm tốn thêm phép toán và chậm đi chương trình
	*/
	if s1Len == 0 {
		return s2Len
	}
	if s2Len == 0 {
		return s1Len
	}
	// Stack array cho strings ngắn (99% trường hợp filename < 64 chars)
	// Go compiler tự stack-allocate fixed-size array, zero heap alloc
	var stackBuf [64]int
	var column []int
	if s1Len+1 <= len(stackBuf) {
		column = stackBuf[:s1Len+1]
	} else {
		column = make([]int, s1Len+1)
	}

	for y := 1; y <= s1Len; y++ {
		column[y] = y
	}
	/*
			Ở đây mình sẽ giải thích sơ sơ
			Thay vì dùng ma trận, mình dùng column như 1 stack từ trên xuống vậy, và ta sẽ ghi đè lên cái nào đã dùng
			Chủ yếu để tiết kiệm 1 chút bộ nhớ thôi
			Giờ nhìn ma trận trước
		        /*
		          "" |  A |  B |  C
		        ┌────┬────┬────┬────┐
		      ""│  0 │  1 │  2 │  3 │   ← khởi tạo, từ rỗng thành rỗng cần 0 bước, thành A cần qua chữ A, thành B cần qua A,B, thành C cần qua A,B,C
		        ├────┼────┼────┼────┤
		      A │  1 │  0 │  1 │  2 │   ← A=A (0), còn lại +1 theo cách biến đổi như cách khởi tạo
		        ├────┼────┼────┼────┤
		      X │  2 │  1 │  ? │  2 │   ← chuỗi AX, đổi thành rỗng cần 2 bước,... nhưng X≠B đọc tiếp xuống dưới
		        ├────┼────┼────┼────┤
		      C │  3 │  2 │  2 │  1 │   ← Tương tự, tại ô của B, Biến AX thành AB (tốn 1 bước sửa X->B), sau đó dư chữ C nên phải Xóa C (1 bước nữa)
		        └────┴────┴────┴────┘
			Kết quả tại ô "?" = 1
			Vì ô ? = min(trên, trái, chéo trái) + 1 (+1 khi ta thấy được ký tự khác nhau)
			Còn bạn nhìn vào ô (4,4) (C,C) ta thấy nó bằng 1 vì min(trên, trái, chéo trái) không + 1 vì C-C giống nhau
			Bây giờ, hãy xem chuyện gì xảy ra khi ta ép cái bảng trên vào 1 mảng duy nhất (column)

	*/

	for i := 1; i <= s2Len; i++ {
		column[0] = i    // Ví dụ: "" -> "A" (1 thêm), "" -> "AX" (2 thêm)
		lastKey := i - 1 // Lưu giá trị cũ của ô chéo trên trái ta đã đề cập
		for j := 1; j <= s1Len; j++ {
			/*
					IMPORTANT: Lưu lại giá trị cũ của column[j] trước khi bị ghi đè
					column[j] lúc này đang chứa giá trị của hàng bên trên (i-1)
					Sau khi vòng lặp này kết thúc, giá trị này sẽ trở thành
				    ô chéo trên trái cho vòng lặp tiếp theo (j+1)
			*/
			oldKey := column[j]
			/*
							Tính toán chi phí biến đổi:

									(lastKey)    (column[j] cũ)
				   					  CHÉO      |     TRÊN
				    				   ↘        |      ↓
				           					┌───────┐
				  				   TRÁI ──→ │  ???  │  (Đang tính)
				              (column[j-1]) └───────┘

							NOTE: lastKey = column[j-1]
			*/
			var incr int
			if s1[j-1] != s2[i-1] {
				incr = 1 // Khác nhau: +1 bước thay thế, còn không thì thôi không cần cộng
			}

			// Và đây chính xác là cái min chúng ta đã làm ở trên: min(trên, trái, chéo trái)
			// Xóa. Ví dụ: Name -> Nam
			minVal := column[j] + 1
			// Thêm. Ví dụ: Nam -> Name
			if column[j-1]+1 < minVal {
				minVal = column[j-1] + 1
			}
			// Sửa. Ví dụ: Năm -> Nấm
			if lastKey+incr < minVal {
				minVal = lastKey + incr
			}
			column[j] = minVal
			// Giá trị Trên của ô hiện tại (oldKey) sẽ trở thành
			// giá trị Chéo của ô bên phải
			lastKey = oldKey
		}
	}
	// Trả về chi phí cuối dựa trên độ dài s1 (phần tử cuối). Đọc tới đây mà không hiểu thì hãy xem lại ma trận
	return column[s1Len]
}
