
#### Benchmark chạy trên Google Cloud VM (N2-Standard-2, Intel® Xeon® 2.80 GHz, 2 vCPU, 8 GB RAM)

| Operation                     | Time         | Memory   | Notes                            |
| :---------------------------- | :----------- | :------- | :------------------------------- |
| **NewSearcher (Linux 100K)**  | **106.07ms** | 56.12MB  | Build index (~100K system files) |
| **Search 100K (Dataset)**     | **2.33ms**   | 515.41KB | Best-case scenario (RealWorld)   |
| **Search 100K (Linux Path)**  | **13.63ms**  | 2.75MB   | Deep nesting (`/usr/lib/...`)    |
| **Search 100K (Typo/Fuzzy)**  | **13.18ms**  | 5.05KB   | Fuzzy match with errors          |
| **Search 50K (Real dataset)** | **1.06ms**   | 203.16KB | Standard workload                |
| **Search with Cache**         | **30.45µs**  | 12.11KB  | Result from internal cache       |
| **Vietnamese (Accents)**      | **32.90µs**  | 11.81KB  | Normalize + Match                |
| **Search 1K (Synthetic)**     | **26.40µs**  | 11.81KB  | Small scale test                 |
| **Search 100 (Synthetic)**    | **4.91µs**   | 9.56KB   | Ultra-fast small scale           |
| **GetBoostScores**            | **21.90µs**  | 6.80KB   | Ranking logic (12 allocs)        |
| **RecordSelection**           | **4.38µs**   | 89B      | 4 allocs                         |
| **Normalize**                 | **920.5ns**  | 80B      | String cleaning (5 allocs)       |
| **LevenshteinRatio**          | **380.8ns**  | 0B       | **Zero allocation core**         |

#### Benchmark chạy trên laptop cá nhân (AMD Ryzen 7 PRO 7840HS, 32 GB RAM)

| Operation                      | Time        | Memory   | Notes                            |
| :----------------------------- | :---------- | :------- | :------------------------------- |
| **NewSearcher (Linux 100K)**   | **80.23ms** | 47.50MB  | Build index (~100K system files) |
| **Search 100K (Real dataset)** | **1.55ms**  | 615.49KB | Best-case scenario (RealWorld)   |
| **Search 100K (Linux Path)**   | **4.32ms**  | 3.42MB   | Deep nesting (`/usr/bin/...`)    |
| **Search 100K (Typo/Fuzzy)**   | **3.90ms**  | 3.43MB   | Fuzzy match with errors          |
| **Search 50K (Real dataset)**  | **0.72ms**  | 247.95KB | Sub-millisecond performance      |
| **Search with Cache**          | **36.83µs** | 12.47KB  | Result from internal cache       |
| **Vietnamese (Accents)**       | **39.68µs** | 12.15KB  | Normalize + Match                |
| **Search 1K (Synthetic)**      | **32.77µs** | 12.15KB  | Small scale test                 |
| **Search 100 (Synthetic)**     | **6.41µs**  | 9.80KB   | Ultra-fast small scale           |
| **GetBoostScores**             | **22.35µs** | 6.96KB   | Ranking logic (12 allocs)        |
| **RecordSelection**            | **3.63µs**  | 89B      | 4 allocs                         |
| **Normalize**                  | **1116ns**  | 80B      | String cleaning (5 allocs)       |
| **LevenshteinRatio**           | **379.8ns** | 0B       | **Zero allocation core**         |


