package io_util

import (
	"io"
	"sync/atomic"
)

type ReadCounter struct {
	reader io.Reader
	count  int64
}

// NewReadCounter 读取字节计数器。获取 reader 被读出的字节数。调用 [ReadCounter.Count] 获取字节数。
// reader 是 nil 将触发恐慌。
func NewReadCounter(reader io.Reader) *ReadCounter {
	if reader == nil {
		panic("reader cannot be nil")
	}
	return &ReadCounter{reader: reader}
}

func (r *ReadCounter) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	atomic.AddInt64(&r.count, int64(n))
	return
}

// Count 获取被读出的字节数。
func (r *ReadCounter) Count() int64 {
	return atomic.LoadInt64(&r.count)
}
