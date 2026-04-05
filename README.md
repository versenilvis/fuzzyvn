<div align="center">
 <img width="20%" width="1920" height="1920" alt="gopher-min" src="https://github.com/user-attachments/assets/a7f7729e-2e34-4ecc-8866-c8c85d93f233" />

  <h1>FuzzyVN</h1>

  [![License: 0BSD](https://img.shields.io/badge/License-0BSD-blue?style=for-the-badge&logo=github&logoColor=white)](./LICENSE.md)
  [![Status](https://img.shields.io/badge/status-beta-yellow?style=for-the-badge&logo=github&logoColor=white)]()
  [![Documentation](https://img.shields.io/badge/docs-available-brightgreen?style=for-the-badge&logo=github&logoColor=white)](./fuzzyvn.go)
  [![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen?style=for-the-badge&logo=github&logoColor=white)](./.github/CONTRIBUTING.md)
</div>
<p><b>FuzzyVN là thư viện tìm kiếm file bằng kỹ thuật chính là fuzzy matching được tối ưu cho tiếng Việt, và còn nhanh hơn với tiếng Anh. Kết hợp nhiều thuật toán tìm kiếm với hệ thống cache thông minh để cho kết quả nhanh và chính xác</b></p>

> [!IMPORTANT]
> **FuzzyVN tập trung vào việc tăng khả năng chính xác khi sai chính tả để đánh đổi một phần tốc độ nhưng vẫn đảm bảo tốc độ cần thiết cho dự án.**  
> Fuzzyvn vẫn hỗ trợ rất tốt cho tiếng Anh.  
> FuzzyVN hỗ trợ tốt nhất cho tìm kiếm file theo file path thay vì chỉ mỗi tên file (Single String), có thể sẽ dùng thừa tài nguyên cũng như điểm số có thể sai lệch một chút (sẽ không ảnh hưởng nhiều).  
> Package này chỉ nên dùng ở local hoặc side project.  
> Vui lòng không được sử dụng trong production.  
> Mình sẽ không chịu bất kỳ trách nhiệm nào khi bạn sử dụng nó.

<br>

<div align="center">
 <img width="1320" height="630" alt="image" src="https://github.com/user-attachments/assets/c26711db-c3cd-4d44-b03a-3915b05a03ee" />
</div>

<div align="center"><i>Bạn có thể test qua phần <a href="https://github.com/versenilvis/fuzzyvn/tree/main/demo">demo</a></i></div>

## Tính năng

- **Tối ưu cho tiếng Việt**
- **Xử lí lỗi chính tả**
- **Đa thuật toán**
- **Hệ thống cache**
- **Thread-Safe**
- **Xử lý parallel cho dataset lớn**

## Cài đặt

```bash
go get github.com/versenilvis/fuzzyvn
```

**Yêu cầu**: Go 1.21+

**Dependencies**: Chỉ cần `golang.org/x/text` để normalize tiếng Việt

## Benchmark
> [!NOTE]
> Benchmark chạy trên Google Cloud VM (N2-Standard-2, Intel® Xeon® 2.80 GHz, 2 vCPU, 8 GB RAM).

| Operation                     | Time         | Memory   | Notes                                     |
| :---------------------------- | :----------- | :------- | :---------------------------------------- |
| **NewSearcher (Linux 100K)**  | **106.07ms** | 56.12MB  | Build index (~100K system files)          |
| **Search 100K (Dataset)**     | **2.33ms**   | 515.41KB | Dataset tự tạo với `make gen` (RealWorld) |
| **Search 100K (Linux Path)**  | **13.63ms**  | 2.75MB   | Deep nesting (`/usr/lib/...`)             |
| **Search 100K (Typo/Fuzzy)**  | **13.18ms**  | 5.05KB   | Search sai chính tả                       |
| **Search 50K (Real dataset)** | **1.06ms**   | 203.16KB | Standard workload                         |
| **Search with Cache**         | **30.45µs**  | 12.11KB  | Có cache                                  |
| **Vietnamese (Accents)**      | **32.90µs**  | 11.81KB  | Normalize + Match                         |
| **Search 1K (Synthetic)**     | **26.40µs**  | 11.81KB  |                                           |
| **Search 100 (Synthetic)**    | **4.91µs**   | 9.56KB   |                                           |
| **GetBoostScores**            | **21.90µs**  | 6.80KB   | Ranking logic (12 allocs)                 |
| **RecordSelection**           | **4.38µs**   | 89B      | 4 allocs                                  |
| **Normalize**                 | **920.5ns**  | 80B      | String cleaning (5 allocs)                |
| **LevenshteinRatio**          | **380.8ns**  | 0B       | **Zero allocation core**                  |

*Test và lấy kết quả trung bình sau 5 lần*

```bash
GOMAXPROCS=1 taskset -c 1 go test -run=^$ -bench=. -benchmem -benchtime=5s -count=5
```
hoặc
```bash
make bench
```
*Make bench không ổn định cho nhiều lần test liên tục, nếu bạn chỉ cần quan tâm test 1 lần*

Full benchmark [tại đây](./docs/bench_result_n2.txt)

## Nếu bạn muốn tự phát triển

### Nhớ tạo data từ demo trước khi chạy (mình không up lên đây vì lí do dung lương)
```bash
make gen
```
> [!WARNING]
> Phải tạo data trước khi chạy nếu không sẽ lỗi
### Chạy demo
```bash
make demo
```
Kết quả:
> Server running at http://localhost:8080  
> Scanning files from directory: ./test_data  
> Indexed 99987 files. Cache: 0 queries

### Test
```bash
make test
```
### Benchmark
```go
make bench
```
hoặc benchmark cụ thể

```go
go test -bench=BenchmarkLevenshteinRatio -benchmem -count=1
```
## Cách dùng

<details open>
  <summary><b>Ví dụ cơ bản</b></summary>
<br>

```go
//go:build ignore

package main

import (
	"fmt"

	"github.com/versenilvis/fuzzyvn"
)

func main() {
	// 1. TẠO SEARCHER
	// Bạn hoàn toàn có thể đọc từ 1 folder, đây chỉ là ví dụ đơn giản
	files := []string{
		"/home/user/Documents/Báo_cáo_tháng_1.pdf",
		"/home/user/Documents/Hợp_đồng_thuê_nhà.docx",
		"/home/user/Music/Sơn_Tùng_MTP.mp3",
		"/home/user/Code/main.go",
		"/home/user/Code/utils.go",
	}

	searcher := fuzzyvn.NewSearcher(files)

	// 2. TÌM KIẾM CƠ BẢN
	fmt.Println("--- Tìm 'bao cao' ---")
	results := searcher.Search("bao cao")
	for _, path := range results {
		fmt.Println("  →", path)
	}
	// Output: /home/user/Documents/Báo_cáo_tháng_1.pdf

	// 3. TÌM KIẾM KHÔNG DẤU
	fmt.Println("\n--- Tìm 'son tung' (không dấu) ---")
	results = searcher.Search("son tung")
	for _, path := range results {
		fmt.Println("  →", path)
	}
	// Output: /home/user/Music/Sơn_Tùng_MTP.mp3

	// 4. SỬA LỖI CHÍNH TẢ (Levenshtein)
	fmt.Println("\n--- Tìm 'maiin' (gõ sai) ---")
	results = searcher.Search("maiin")
	for _, path := range results {
		fmt.Println("  →", path)
	}
	// Output: /home/user/Code/main.go

	// 5. FILE MEMORY - Học hành vi người dùng (Frecency)
	fmt.Println("\n--- Memory Demo ---")

	// User tìm "main" và chọn main.go (lưu lại lịch sử)
	searcher.RecordSelection("main", "/home/user/Code/main.go")
	searcher.RecordSelection("main", "/home/user/Code/main.go")

	// Lần tìm kiếm sau, file này sẽ được ưu tiên lên top 1 ngay lập tức
	fmt.Println("Tìm 'mai' (sau khi đã record):")
	results = searcher.Search("mai")
	for _, path := range results {
		fmt.Println("  →", path)
	}

	// 6. XÓA BỘ NHỚ LỊCH SỬ
	searcher.ClearCache()
	fmt.Println("\n✓ Đã xóa bộ nhớ lịch sử!")
}
```
</details>

<details>
  <summary><b>Ví dụ với Memory (Frecency)</b></summary>
<br>


```go
// Người dùng tìm kiếm
results := searcher.Search("main")

// Người dùng chọn file (ví dụ chọn kết quả thứ 3)
selectedFile := results[2]
searcher.RecordSelection("main", selectedFile)

// Lần tìm kiếm sau với query tương tự, file này được đẩy lên đầu
results = searcher.Search("mai") 
```
</details>

<details>
  <summary><b>Ví dụ với HTTP Server</b></summary>
<br>

Xem ví dụ ở [demo](https://github.com/versenilvis/fuzzyvn/tree/main/demo)
</details>

## Tài liệu

### API chính

#### `NewSearcher(items []string) *Searcher`
Tạo searcher mới từ danh sách file paths.

#### `Search(query string, opts ...*SearchOptions) []string`
Tìm kiếm và trả về top 20 kết quả phù hợp nhất. Tự động áp dụng điểm cộng từ lịch sử (Memory).

#### `RecordSelection(query, filePath string)`
Ghi nhận file người dùng đã chọn để tăng độ ưu tiên (Frecency) cho các lần tìm kiếm sau.

#### `ClearCache()`
Xóa toàn bộ lịch sử tìm kiếm và lựa chọn của người dùng.

### Utility Functions

```go
// Normalize string (bỏ dấu tiếng Việt)
normalized := fuzzyvn.Normalize("Tiếng Việt")
// Output: "tieng viet"

// Tính khoảng cách Levenshtein
distance := fuzzyvn.LevenshteinRatio("hello", "mian")
```

**Ví dụ**:
- User search `"màn hình"` → chọn `"dell-monitor.pdf"`
- User search `"man hinh"` → `"dell-monitor.pdf"` lên top nhờ Frecency boost.

## Các trường hợp sử dụng

<details>
  <summary><b>1. File Explorer / Launcher</b></summary>
<br>

```go
// Quét thư mục
files := scanDirectory("/home/user")
searcher := fuzzyvn.NewSearcher(files)

// Tìm kiếm realtime
results := searcher.Search(userInput)
```

</details>

<details>
  <summary><b>2. Code Search</b></summary>
<br>
 
```go
// Index source code
code := scanIgnoreDirs("/project", []string{"node_modules", ".git"})
searcher := fuzzyvn.NewSearcher(code)

// Tìm file mian -> vẫn ra main.go nhờ typo tolerance & transposition handling
results := searcher.Search("mian")
```

</details>

## Ví dụ nâng cao

<details>
  <summary><b>Giữ lại Memory khi Rebuild Index</b></summary>
<br>
 
```go
func watchAndRebuild(searcher **fuzzyvn.Searcher) {
    watcher := setupFileWatcher()

    for event := range watcher.Events {
        // Giữ lại memory cũ để không mất lịch sử người dùng
        oldMemory := (*searcher).Memory

        // Quét lại files mới
        newFiles := scanDirectory("/project")

        // Khởi tạo searcher mới với bộ nhớ cũ
        *searcher = fuzzyvn.NewSearcherWithMemory(newFiles, oldMemory)
    }
}
```

</details>

## Đóng góp

Vui lòng theo chuẩn [Contributing](.github/CONTRIBUTING.md) khi tạo một contribution qua pull request.

## Giấy phép

Package này được cấp phép bởi giấy phép [0BSD License](LICENSE). Bạn có thể sửa, xóa, thêm hay làm bất cứ thứ gì bạn muốn với nó.
