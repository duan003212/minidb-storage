package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// ==========================================
// 1. 数据协议定义 (Data Protocol)
// ==========================================

const (
	HeaderSize    = 16
	DBFileName    = "minidb.data"
	MergeFileName = "minidb.data.merge"
)

type Entry struct {
	Key       []byte
	Value     []byte
	KeySize   uint32
	ValueSize uint32
	Timestamp uint32 // 记录写入时间
	CRC       uint32 // 校验码
}

func NewEntry(key, value []byte) *Entry {
	return &Entry{
		Key:       key,
		Value:     value,
		KeySize:   uint32(len(key)),
		ValueSize: uint32(len(value)),
		Timestamp: uint32(time.Now().Unix()),
	}
}

func (e *Entry) Encode() []byte {
	buf := make([]byte, HeaderSize+e.KeySize+e.ValueSize)

	binary.BigEndian.PutUint32(buf[4:8], e.Timestamp)
	binary.BigEndian.PutUint32(buf[8:12], e.KeySize)
	binary.BigEndian.PutUint32(buf[12:16], e.ValueSize)
	copy(buf[HeaderSize:], e.Key)
	copy(buf[HeaderSize+e.KeySize:], e.Value)

	crc := crc32.ChecksumIEEE(buf[4:])
	binary.BigEndian.PutUint32(buf[0:4], crc)

	return buf
}

func DecodeHeader(buf []byte) (uint32, uint32, uint32, uint32) {
	crc := binary.BigEndian.Uint32(buf[0:4])
	ts := binary.BigEndian.Uint32(buf[4:8])
	kSize := binary.BigEndian.Uint32(buf[8:12])
	vSize := binary.BigEndian.Uint32(buf[12:16])
	return crc, ts, kSize, vSize
}

// ==========================================
// 2. 存储引擎实现 (Storage Engine)
// ==========================================

type MiniDB struct {
	mu      sync.RWMutex
	file    *os.File
	indexes map[string]int64
	offset  int64
}

func Open() (*MiniDB, error) {
	db := &MiniDB{
		indexes: make(map[string]int64),
	}

	if err := db.initFile(); err != nil {
		return nil, err
	}

	if err := db.loadIndexes(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *MiniDB) initFile() error {
	file, err := os.OpenFile(DBFileName, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	db.file = file
	stat, _ := file.Stat()
	db.offset = stat.Size()
	return nil
}

func (db *MiniDB) loadIndexes() error {
	log.Println("Loading indexes from disk...")

	f, err := os.Open(DBFileName)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	var offset int64 = 0

	for {
		header := make([]byte, HeaderSize)
		_, err := io.ReadFull(reader, header)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		crc, _, kSize, vSize := DecodeHeader(header)

		payloadSize := int64(kSize + vSize)
		payload := make([]byte, payloadSize)
		_, err = io.ReadFull(reader, payload)
		if err != nil {
			return err
		}

		checkBuf := append(header[4:], payload...)
		if crc32.ChecksumIEEE(checkBuf) != crc {
			log.Printf("Warn: Corrupted data at offset %d, skipping...", offset)
		} else {
			key := string(payload[:kSize])
			db.indexes[key] = offset
		}

		offset += HeaderSize + payloadSize
	}
	log.Printf("Index loaded. Total keys: %d", len(db.indexes))
	return nil
}

func (db *MiniDB) Put(key string, value string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	entry := NewEntry([]byte(key), []byte(value))
	data := entry.Encode()

	n, err := db.file.Write(data)
	if err != nil {
		return err
	}

	db.indexes[key] = db.offset
	db.offset += int64(n)
	return nil
}

func (db *MiniDB) Get(key string) (string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	offset, ok := db.indexes[key]
	if !ok {
		return "", errors.New("key not found")
	}

	header := make([]byte, HeaderSize)
	_, err := db.file.ReadAt(header, offset)
	if err != nil {
		return "", err
	}

	crc, _, kSize, vSize := DecodeHeader(header)

	body := make([]byte, kSize+vSize)
	_, err = db.file.ReadAt(body, offset+HeaderSize)
	if err != nil {
		return "", err
	}

	checkBuf := append(header[4:], body...)
	if crc32.ChecksumIEEE(checkBuf) != crc {
		return "", errors.New("data corrupted")
	}

	return string(body[kSize:]), nil
}

func (db *MiniDB) Del(key string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	delete(db.indexes, key)
}

func (db *MiniDB) Merge() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	log.Println("Starting merge process...")

	mergeFile, err := os.OpenFile(MergeFileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer mergeFile.Close()

	newIndexes := make(map[string]int64)
	var newOffset int64 = 0

	for key, oldOffset := range db.indexes {
		header := make([]byte, HeaderSize)
		db.file.ReadAt(header, oldOffset)
		_, _, kSize, vSize := DecodeHeader(header)

		raw := make([]byte, HeaderSize+kSize+vSize)
		db.file.ReadAt(raw, oldOffset)

		n, err := mergeFile.Write(raw)
		if err != nil {
			return err
		}

		newIndexes[key] = newOffset
		newOffset += int64(n)
	}

	db.file.Close()
	os.Remove(DBFileName)
	os.Rename(MergeFileName, DBFileName)

	file, err := os.OpenFile(DBFileName, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	db.file = file
	db.indexes = newIndexes
	db.offset = newOffset

	log.Printf("Merge complete. Reclaimed space. New file size: %d", newOffset)
	return nil
}

func (db *MiniDB) Close() {
	db.file.Close()
}

// ==========================================
// 3. HTTP 接口
// ==========================================

var db *MiniDB

func main() {
	var err error
	db, err = Open()
	if err != nil {
		log.Fatalf("Init DB failed: %v", err)
	}
	defer db.Close()

	http.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		val := r.URL.Query().Get("value")
		if key == "" {
			http.Error(w, "key required", 400)
			return
		}

		if err := db.Put(key, val); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		fmt.Fprint(w, "OK")
	})

	http.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		val, err := db.Get(key)
		if err != nil {
			http.Error(w, "not found or error", 404)
			return
		}
		fmt.Fprint(w, val)
	})

	http.HandleFunc("/del", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		db.Del(key)
		fmt.Fprint(w, "OK")
	})

	http.HandleFunc("/merge", func(w http.ResponseWriter, r *http.Request) {
		go func() {
			if err := db.Merge(); err != nil {
				log.Printf("Merge failed: %v", err)
			}
		}()
		fmt.Fprint(w, "Merge task started")
	})

	log.Println("Server running at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
