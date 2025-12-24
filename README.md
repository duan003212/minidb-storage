# MiniDB: A High-Performance Persistent KV Store

[![Go Report Card](https://goreportcard.com/badge/github.com/yourusername/minidb)](https://goreportcard.com/report/github.com/yourusername/minidb)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/yourusername/minidb)](go.mod)

> 一个基于 **Bitcask** 模型（Log-Structured Hash Table）实现的轻量级、高性能持久化 KV 存储引擎。
>
> A lightweight, high-performance persistent key-value storage engine based on the **Bitcask** model, written in Go.

## 📖 Introduction (项目介绍)

**MiniDB** 是一个为了深入理解数据库底层原理而实现的 KV 存储引擎。它采用了类似于 Riak 早期版本的 **Bitcask** 架构，核心设计思想是 **顺序 IO (Sequential I/O)** 写入和 **内存索引 (In-Memory Index)** 读取。

该项目展示了如何处理文件系统 IO、二进制数据编解码、数据完整性校验以及并发控制等核心后端技术。

**Key Features:**
*   🚀 **高性能写入**: 采用 Log-Structured (Append Only) 模式，利用顺序 IO 获得极高的写入吞吐量。
*   ⚡ **O(1) 读取**: 维护内存 Hash 索引，单次磁盘寻址即可获取数据。
*   🛡️ **数据可靠性**: 实现了 CRC32 校验机制，防止磁盘静默错误导致的数据损坏。
*   🧹 **数据压缩 (Compaction)**: 实现了 Log Merge 机制，定期清理无效的历史数据，防止磁盘空间无限膨胀。
*   🔒 **并发安全**: 支持多线程并发读写 (Thread-Safe)。

## ⚙️ Architecture (架构设计)

MiniDB 的核心架构包含以下几个部分：

1.  **Write Process**: 所有写入操作（Put/Delete）都以追加方式写入活跃数据文件，格式为 `[CRC][Timestamp][KeySize][ValueSize][Key][Value]`。
2.  **Read Process**: 启动时扫描数据文件建立内存索引 `Key -> (FileOffset, ValueSize)`。读取时通过索引定位，仅需一次磁盘 Seek。
3.  **Crash Recovery**: 利用 Write-Ahead Log (WAL) 的思想，重启时自动重放日志恢复索引。
4.  **Compaction**: 针对 Bitcask 模型“只增不减”的问题，实现了后台 Merge 线程，重写有效数据并移除 Tombstone 记录。

## 🛠️ Getting Started (快速开始)

### Prerequisites
*   Go 1.18+

### Installation

```bash
git clone https://github.com/yourusername/minidb.git
cd minidb
go mod init minidb # 如果还没初始化
go run main.go
```

### Usage (HTTP API)

MiniDB 默认运行在 `:8080` 端口。

#### 1. 写入数据 (Set)
```bash
curl "http://localhost:8080/set?key=language&value=golang"
# Output: OK
```

#### 2. 读取数据 (Get)
```bash
curl "http://localhost:8080/get?key=language"
# Output: golang
```

#### 3. 删除数据 (Delete)
```bash
curl "http://localhost:8080/del?key=language"
# Output: OK
```

#### 4. 手动触发合并 (Merge/Compact)
```bash
curl "http://localhost:8080/merge"
# Output: Merge task started
```

## 📝 Performance & Optimization (优化细节)

在实现过程中，特别针对以下痛点进行了优化：

*   **Binary Protocol**: 自定义了紧凑的二进制存储协议，相比 JSON/Text 格式减少了存储空间并提升了解析速度。
*   **Safety**: 引入 `CRC32` 校验，在 `Get` 和 `Load` 阶段验证数据，确保数据一致性。
*   **Space Reclamation**: 通过 `Merge` 接口，将分散的旧数据文件合并为紧凑的新文件，释放磁盘空间。

## 🔜 Future Roadmap (未来规划)

*   [ ] 支持 Hint File 索引文件，加速启动时的索引构建速度。
*   [ ] 引入 Bloom Filter (布隆过滤器) 减少对不存在 Key 的磁盘读取。
*   [ ] 支持 Redis 协议 (RESP)，使其兼容 redis-cli。
*   [ ] 支持 Key 的 TTL (过期时间)。

## 📄 License

MIT License
