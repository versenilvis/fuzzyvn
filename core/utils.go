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
	// 1. FAST PATH: Nếu toàn là ASCII (Tiếng Anh, Code) -> Lowercase và trả về ngay
	// Đây là trường hợp phổ biến nhất (90% file source code) -> Tốc độ siêu nhanh
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

	// 2. NFD CHECK: Chỉ convert về NFC nếu chuỗi đang ở dạng NFD (thường gặp trên macOS)
	// Hàm IsNormalString rất nhanh, giúp tránh việc allocate lại chuỗi nếu không cần thiết
	if !norm.NFC.IsNormalString(s) {
		s = norm.NFC.String(s)
	}

	// 3. BUFFER: dùng []byte trực tiếp, nhanh hơn strings.Builder
	buf := make([]byte, 0, len(s))

	// 4. MANUAL MAPPING: Duyệt từng rune và map thủ công
	for _, r := range s {
		r = unicode.ToLower(r)

		switch r {
		case 'á', 'à', 'ả', 'ã', 'ạ', 'ă', 'ắ', 'ằ', 'ẳ', 'ẵ', 'ặ', 'â', 'ấ', 'ầ', 'ẩ', 'ẫ', 'ậ':
			buf = append(buf, 'a')
		case 'đ':
			buf = append(buf, 'd')
		case 'é', 'è', 'ẻ', 'ẽ', 'ẹ', 'ê', 'ế', 'ề', 'ể', 'ễ', 'ệ':
			buf = append(buf, 'e')
		case 'í', 'ì', 'ỉ', 'ĩ', 'ị':
			buf = append(buf, 'i')
		case 'ó', 'ò', 'ỏ', 'õ', 'ọ', 'ô', 'ố', 'ồ', 'ổ', 'ỗ', 'ộ', 'ơ', 'ớ', 'ờ', 'ở', 'ỡ', 'ợ':
			buf = append(buf, 'o')
		case 'ú', 'ù', 'ủ', 'ũ', 'ụ', 'ư', 'ứ', 'ừ', 'ử', 'ữ', 'ự':
			buf = append(buf, 'u')
		case 'ý', 'ỳ', 'ỷ', 'ỹ', 'ỵ':
			buf = append(buf, 'y')
		default:
			if r < 128 {
				buf = append(buf, byte(r))
			} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
				var tmp [4]byte
				n := utf8.EncodeRune(tmp[:], r)
				buf = append(buf, tmp[:n]...)
			}
		}
	}
	return string(buf)
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
