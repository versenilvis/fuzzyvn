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

| Operation                  | Time        | Memory | Notes                            |
| -------------------------- | ----------- | ------ | -------------------------------- |
| NewSearcher                | **0.282ms** | 330KB  | Build index (synthetic 1K files) |
| Search 100 files           | **23µs**    | 27KB   | Synthetic                        |
| Search 1K files            | **203µs**   | 74KB   | Synthetic                        |
| Search 10K files           | **3.50ms**  | 191KB  | Synthetic (1 outlier)            |
| Search 50K files           | **20.95ms** | 338KB  | Real dataset                     |
| Search 100K files          | **41.74ms** | 720KB  | Real dataset                     |
| Search 100K files, typo    | **44.26ms** | 652KB  | Fuzzy match                      |
| Vietnamese with accents    | **395µs**   | 52KB   | Normalize + match                |
| Vietnamese without accents | **394µs**   | 52KB   | Normalize + match                |
| Search with cache          | **208µs**   | 73KB   | Boost ranking                    |
| Normalize                  | **1.28µs**  | 112B   | 9 allocs                         |
| LevenshteinRatio           | **307ns**   | 0B     | Zero allocation                  |
| RecordSelection            | **372ns**   | 29B    | 2 allocs                         |
| GetBoostScores             | **33.6µs**  | 9.8KB  | 207 allocs                       |

```bash
GOMAXPROCS=1 taskset -c 1 go test -run=^$ -bench=. -benchmem -benchtime=5s -count=5
```
hoặc
```bash
make bench
```
*Make bench không ổn định cho nhiều lần test liên tục, nếu bạn chỉ cần quan tâm test 1 lần*

<details>
	<summary>Full benchmark</summary>

```
--- Benchmark Info ---
Đã load 99992 files từ ổ cứng
----------------------
goos: linux
goarch: amd64
pkg: github.com/versenilvis/fuzzyvn
cpu: Intel(R) Xeon(R) CPU @ 2.80GHz
BenchmarkSearch_RealWorld/Search/50k_Files                   286          20856288 ns/op          345724 B/op         50 allocs/op
BenchmarkSearch_RealWorld/Search/50k_Files                   288          20804539 ns/op          345720 B/op         50 allocs/op
BenchmarkSearch_RealWorld/Search/50k_Files                   285          20930813 ns/op          345720 B/op         50 allocs/op
BenchmarkSearch_RealWorld/Search/50k_Files                   285          20952578 ns/op          345720 B/op         50 allocs/op
BenchmarkSearch_RealWorld/Search/50k_Files                   286          20918743 ns/op          345720 B/op         50 allocs/op
BenchmarkSearch_RealWorld/Search/100k_Files                  144          41576191 ns/op          736920 B/op         53 allocs/op
BenchmarkSearch_RealWorld/Search/100k_Files                  144          41627880 ns/op          736920 B/op         53 allocs/op
BenchmarkSearch_RealWorld/Search/100k_Files                  142          41723329 ns/op          736920 B/op         53 allocs/op
BenchmarkSearch_RealWorld/Search/100k_Files                  142          41736543 ns/op          736920 B/op         53 allocs/op
BenchmarkSearch_RealWorld/Search/100k_Files                  142          41683405 ns/op          736920 B/op         53 allocs/op
BenchmarkSearch_RealWorld/Search/100K_Files_Typo             135          44145793 ns/op          667912 B/op         52 allocs/op
BenchmarkSearch_RealWorld/Search/100K_Files_Typo             135          44065497 ns/op          667912 B/op         52 allocs/op
BenchmarkSearch_RealWorld/Search/100K_Files_Typo             135          44135146 ns/op          667912 B/op         52 allocs/op
BenchmarkSearch_RealWorld/Search/100K_Files_Typo             134          44219059 ns/op          667912 B/op         52 allocs/op
BenchmarkSearch_RealWorld/Search/100K_Files_Typo             134          44264621 ns/op          667912 B/op         52 allocs/op
BenchmarkNewSearcher                                       21714            280562 ns/op          330736 B/op       2013 allocs/op
BenchmarkNewSearcher                                       21277            281646 ns/op          330736 B/op       2013 allocs/op
BenchmarkNewSearcher                                       21171            280012 ns/op          330736 B/op       2013 allocs/op
BenchmarkNewSearcher                                       21564            277925 ns/op          330736 B/op       2013 allocs/op
BenchmarkNewSearcher                                       21510            278624 ns/op          330736 B/op       2013 allocs/op
BenchmarkSearch/100_files                                 260863             23076 ns/op           27040 B/op         28 allocs/op
BenchmarkSearch/100_files                                 262005             22977 ns/op           27040 B/op         28 allocs/op
BenchmarkSearch/100_files                                 255213             23098 ns/op           27040 B/op         28 allocs/op
BenchmarkSearch/100_files                                 261056             23025 ns/op           27040 B/op         28 allocs/op
BenchmarkSearch/100_files                                 261691             23007 ns/op           27040 B/op         28 allocs/op
BenchmarkSearch/1000_files                                 29988            202229 ns/op           74224 B/op        144 allocs/op
BenchmarkSearch/1000_files                                 29415            203169 ns/op           74224 B/op        144 allocs/op
BenchmarkSearch/1000_files                                 29596            202583 ns/op           74224 B/op        144 allocs/op
BenchmarkSearch/1000_files                                 29707            202075 ns/op           74224 B/op        144 allocs/op
BenchmarkSearch/1000_files                                 29800            200921 ns/op           74224 B/op        144 allocs/op
BenchmarkSearch/10000_files                                 2053           2925966 ns/op          195672 B/op       1050 allocs/op
BenchmarkSearch/10000_files                                 2040           2927483 ns/op          195672 B/op       1050 allocs/op
BenchmarkSearch/10000_files                                 2026           3496008 ns/op          195672 B/op       1050 allocs/op
BenchmarkSearch/10000_files                                 2053           2920354 ns/op          195672 B/op       1050 allocs/op
BenchmarkSearch/10000_files                                 2032           2940439 ns/op          195672 B/op       1050 allocs/op
BenchmarkSearchVietnamese/tiếng_Việt_có_dấu                15255            393876 ns/op           53744 B/op        145 allocs/op
BenchmarkSearchVietnamese/tiếng_Việt_có_dấu                15200            394326 ns/op           53744 B/op        145 allocs/op
BenchmarkSearchVietnamese/tiếng_Việt_có_dấu                15199            394632 ns/op           53744 B/op        145 allocs/op
BenchmarkSearchVietnamese/tiếng_Việt_có_dấu                15228            394102 ns/op           53744 B/op        145 allocs/op
BenchmarkSearchVietnamese/tiếng_Việt_có_dấu                15255            393487 ns/op           53744 B/op        145 allocs/op
BenchmarkSearchVietnamese/tiếng_Việt_không_dấu                     15254            393289 ns/op           53712 B/op        141 allocs/op
BenchmarkSearchVietnamese/tiếng_Việt_không_dấu                     15288            392234 ns/op           53712 B/op        141 allocs/op
BenchmarkSearchVietnamese/tiếng_Việt_không_dấu                     15238            394197 ns/op           53712 B/op        141 allocs/op
BenchmarkSearchVietnamese/tiếng_Việt_không_dấu                     15270            392448 ns/op           53712 B/op        141 allocs/op
BenchmarkSearchVietnamese/tiếng_Việt_không_dấu                     15207            394116 ns/op           53712 B/op        141 allocs/op
BenchmarkSearchWithCache                                           29049            208234 ns/op           74464 B/op        147 allocs/op
BenchmarkSearchWithCache                                           28693            208742 ns/op           74464 B/op        147 allocs/op
BenchmarkSearchWithCache                                           28744            206947 ns/op           74464 B/op        147 allocs/op
BenchmarkSearchWithCache                                           29205            206766 ns/op           74464 B/op        147 allocs/op
BenchmarkSearchWithCache                                           29156            205548 ns/op           74464 B/op        147 allocs/op
BenchmarkNormalize                                               4723686              1270 ns/op             112 B/op          9 allocs/op
BenchmarkNormalize                                               4719566              1272 ns/op             112 B/op          9 allocs/op
BenchmarkNormalize                                               4702352              1276 ns/op             112 B/op          9 allocs/op
BenchmarkNormalize                                               4720912              1271 ns/op             112 B/op          9 allocs/op
BenchmarkNormalize                                               4632069              1280 ns/op             112 B/op          9 allocs/op
BenchmarkLevenshteinRatio                                       19559318               306.7 ns/op             0 B/op          0 allocs/op
BenchmarkLevenshteinRatio                                       19470264               307.2 ns/op             0 B/op          0 allocs/op
BenchmarkLevenshteinRatio                                       18950019               307.6 ns/op             0 B/op          0 allocs/op
BenchmarkLevenshteinRatio                                       19590055               305.9 ns/op             0 B/op          0 allocs/op
BenchmarkLevenshteinRatio                                       19526402               306.4 ns/op             0 B/op          0 allocs/op
BenchmarkRecordSelection                                        16344829               367.0 ns/op            29 B/op          2 allocs/op
BenchmarkRecordSelection                                        16391042               367.3 ns/op            29 B/op          2 allocs/op
BenchmarkRecordSelection                                        16248507               368.4 ns/op            29 B/op          2 allocs/op
BenchmarkRecordSelection                                        16292912               368.6 ns/op            29 B/op          2 allocs/op
BenchmarkRecordSelection                                        16262018               372.2 ns/op            29 B/op          2 allocs/op
BenchmarkGetBoostScores                                           192088             32470 ns/op           10088 B/op        207 allocs/op
BenchmarkGetBoostScores                                           177350             33597 ns/op           10088 B/op        207 allocs/op
BenchmarkGetBoostScores                                           183049             33521 ns/op           10088 B/op        207 allocs/op
BenchmarkGetBoostScores                                           176958             33535 ns/op           10088 B/op        207 allocs/op
BenchmarkGetBoostScores                                           178576             33578 ns/op           10088 B/op        207 allocs/op
```
</details>

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

	// 5. CACHE SYSTEM - Học hành vi người dùng
	fmt.Println("\n--- Cache Demo ---")

	// User tìm "main" và chọn main.go
	searcher.RecordSelection("main", "/home/user/Code/main.go")

	// Chọn thêm 2 lần nữa
	searcher.RecordSelection("main", "/home/user/Code/main.go")
	searcher.RecordSelection("main", "/home/user/Code/main.go")

	// Giờ tìm với từ tương tự → main.go được boost lên top
	fmt.Println("Tìm 'mai' (sau khi đã cache):")
	results = searcher.Search("mai")
	for _, path := range results {
		fmt.Println("  →", path)
	}
	// main.go sẽ lên đầu vì đã được chọn 3 lần

	// 6. XEM THỐNG KÊ CACHE
	cache := searcher.GetCache()

	fmt.Println("\n--- Thống kê ---")
	fmt.Println("Recent queries:", cache.GetRecentQueries(3))
	fmt.Println("Recent files:", cache.GetAllRecentFiles(3))
	fmt.Printf("Tổng queries: %d\n", cache.Size())

	// 7. TÙY CHỈNH CACHE
	cache.SetBoostScore(10000) // Tăng độ ưu tiên cho cache
	cache.SetMaxQueries(200)   // Lưu nhiều queries hơn

	fmt.Println("\n✓ Đã cấu hình cache!")
}
```
</details>

<details>
  <summary><b>Ví dụ với Cache</b></summary>
<br>


```go
// Người dùng tìm kiếm
results := searcher.Search("main")

// Người dùng chọn file
selectedFile := results[0]
searcher.RecordSelection("main", selectedFile)

// Lần tìm kiếm sau, file này được ưu tiên
results = searcher.Search("mai")  // Gõ sai, vẫn lên đầu nhờ cache
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
Tạo searcher mới từ danh sách file paths

```go
searcher := fuzzyvn.NewSearcher(files)
```

#### `Search(query string) []string`
Tìm kiếm và trả về top 20 kết quả phù hợp nhất (hardcode 20)

```go
results := searcher.Search("readme")
```

#### `RecordSelection(query, filePath string)`
Lưu lại file mà người dùng đã chọn để cải thiện kết quả tương lai

```go
searcher.RecordSelection("main", "/project/main.go")
```

#### `GetCache() *QueryCache`
Lấy cache object để tùy chỉnh hoặc xem thống kê

```go
cache := searcher.GetCache()
cache.SetBoostScore(10000)      // Tăng boost
cache.SetMaxQueries(500)        // Lưu nhiều query hơn
recentQueries := cache.GetRecentQueries(10)
```

### QueryCache Methods

```go
cache := searcher.GetCache()

// Cấu hình
cache.SetBoostScore(score int)        // Mặc định: 5000
cache.SetMaxQueries(n int)            // Mặc định: 100

// Thống kê
cache.GetRecentQueries(limit int) []string
cache.GetAllRecentFiles(limit int) []string
cache.GetCachedFiles(query string, limit int) []string
cache.Size() int
cache.Clear()
```

### Utility Functions

```go
// Normalize string (bỏ dấu tiếng Việt)
normalized := fuzzyvn.Normalize("Tiếng Việt")
// Output: "Tieng Viet"

// Tính khoảng cách Levenshtein
distance := fuzzyvn.LevenshteinRatio("hello", "helo")
// Output: 1

// Fuzzy find trong slice
matches := fuzzyvn.FuzzyFind("pattern", targets)
```

**Ví dụ**:
- User search `"màn hình"` → chọn `"dell-monitor.pdf"`
- User search `"man hinh"` → `"dell-monitor.pdf"` lên top (similarity 95%)
- User search `"màn hình dell"` → vẫn boost (contains)

## Các trường hợp sử dụng

<details>
  <summary><b>1. File Explorer / Launcher</b></summary>
<br>

```go
// Quét thư mục home
files := scanDirectory("/home/user")
searcher := fuzzyvn.NewSearcher(files)

// User gõ, realtime searchautomatically
results := searcher.Search(userInput)
```

</details>

<details>
  <summary><b>2. Document Management</b></summary>
<br>
 
```go
// Index tài liệu công ty
docs := scanWithExtensions("/company/docs", []string{".pdf", ".docx"})
searcher := fuzzyvn.NewSearcher(docs)

// Tìm hợp đồng
contracts := searcher.Search("hop dong")
```

</details>

<details>
  <summary><b>3. Code Search</b></summary>
<br>
 
```go
// Index source code
code := scanIgnoreDirs("/project", []string{"node_modules", ".git"})
searcher := fuzzyvn.NewSearcher(code)

// Tìm file main
mains := searcher.Search("main")
```

</details>


<details>
  <summary><b>4. Media Library</b></summary>
<br>
 
```go
// Index nhạc
music := scanWithExtensions("/music", []string{".mp3", ".flac"})
searcher := fuzzyvn.NewSearcher(music)

// Tìm bài hát
songs := searcher.Search("son tung")
```

</details>

## Ví dụ nâng cao

<details>
  <summary><b>Rebuild Index khi file thay đổi</b></summary>
<br>
 
```go
func watchAndRebuild(searcher **fuzzyvn.Searcher) {
    watcher := setupFileWatcher()

    for event := range watcher.Events {
        // Giữ lại cache
        cache := (*searcher).GetCache()

        // Quét lại
        newFiles := scanDirectory("/data")

        // Rebuild với cache cũ
        *searcher = fuzzyvn.NewSearcherWithCache(newFiles, cache)
    }
}
```

</details>

<details>
  <summary><b>Tùy chỉnh cho domain cụ thể</b></summary>
<br>
 
```go
searcher := fuzzyvn.NewSearcher(files)
cache := searcher.GetCache()

// Tăng boost cho người dùng power user
cache.SetBoostScore(15000)

// Lưu nhiều lịch sử hơn
cache.SetMaxQueries(1000)
```

</details>

<details>
  <summary><b>Integration với CLI tool</b></summary>
<br>
 
```go
func main() {
    files := scanDirectory(os.Getenv("HOME"))
    searcher := fuzzyvn.NewSearcher(files)

    reader := bufio.NewReader(os.Stdin)
    for {
        fmt.Print("Search> ")
        query, _ := reader.ReadString('\n')
        query = strings.TrimSpace(query)

        results := searcher.Search(query)
        for i, r := range results {
            fmt.Printf("[%d] %s\n", i, r)
        }

        fmt.Print("Select> ")
        input, _ := reader.ReadString('\n')
        idx, _ := strconv.Atoi(strings.TrimSpace(input))

        if idx >= 0 && idx < len(results) {
            searcher.RecordSelection(query, results[idx])
            // Open file...
        }
    }
}
```

</details>

## Đóng góp

Vui lòng theo chuẩn [Contributing](.github/CONTRIBUTING.md) khi tạo một contribution qua pull request.

## Giấy phép

Package này được cấp phép bởi giấy phép [0BSD License](LICENSE). Bạn có thể sửa, xóa, thêm hay làm bất cứ thứ gì bạn muốn với nó.
