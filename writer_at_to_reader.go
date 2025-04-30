/*
 * Copyright (c) 2023 ivfzhou
 * io-util is licensed under Mulan PSL v2.
 * You can use this software according to the terms and conditions of the Mulan PSL v2.
 * You may obtain a copy of Mulan PSL v2 at:
 *          http://license.coscl.org.cn/MulanPSL2
 * THIS SOFTWARE IS PROVIDED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OF ANY KIND,
 * EITHER EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO NON-INFRINGEMENT,
 * MERCHANTABILITY OR FIT FOR A PARTICULAR PURPOSE.
 * See the Mulan PSL v2 for more details.
 */

package io_util

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
)

var (
	ErrOffsetCannotNegative = errors.New("offset cannot negative")
	ErrWriterIsClosed       = errors.New("writer is closed")
	ErrReaderIsClosed       = errors.New("reader is closed")
)

type WriteAtCloser interface {
	io.WriterAt
	io.Closer

	// CloseByError Deprecated: 使用 Close 代替。
	CloseByError(error) error
}

type readCloser struct {
	*writerAtToReader
}

type writeCloser struct {
	*writerAtToReader
}

type writerAtToReader struct {
	segments                         *SegmentManager
	lock                             sync.Mutex
	writerCloseFlag, readerCloseFlag int32
}

// NewWriteAtToReader 获取一个 WriteAtCloser 和 io.ReadCloser 对象，其中 wc 用于并发写入数据，而与此同时 rc 对象同时读取出已经写入好的数据。
//
// wc 写入完毕后调用 Close，则 rc 会全部读取完后返回 io.EOF。
//
// wc 发生的 error 会传递给 rc 返回。
func NewWriteAtToReader() (wc WriteAtCloser, rc io.ReadCloser) {
	w := &writerAtToReader{segments: &SegmentManager{}}
	return &writeCloser{w}, &readCloser{w}
}

// NewWriteAtReader 获取一个WriterAt和Reader对象，其中WriterAt用于并发写入数据，而与此同时Reader对象同时读取出已经写入好的数据。
//
// WriterAt写入完毕后调用Close，则Reader会全部读取完后结束读取。
//
// WriterAt发生的error会传递给Reader返回。
//
// 该接口是特定为一个目的实现————服务器分片下载数据中转给客户端下载，提高中转数据效率。
//
// Deprecated: 使用 NewWriteAtToReader 代替。
func NewWriteAtReader() (WriteAtCloser, io.ReadCloser) {
	return NewWriteAtToReader()
}

func (m *writerAtToReader) WriteAt(bs []byte, offset int64) (writtenLength int, err error) {
	if offset < 0 {
		return 0, ErrOffsetCannotNegative
	}
	if len(bs) <= 0 {
		return
	}

	if atomic.LoadInt32(&m.writerCloseFlag) > 0 {
		return 0, ErrWriterIsClosed
	}

	// 已经关闭了读取流，则不必再关心写入数据。
	writtenLength = len(bs)
	if atomic.LoadInt32(&m.readerCloseFlag) > 0 {
		return
	}

	// 将字节数据保存。
	return m.segments.WriteAt(bs, offset)
}

func (m *writerAtToReader) Read(p []byte) (int, error) {
	// 已经关闭了就不能再读取流。
	if atomic.LoadInt32(&m.readerCloseFlag) > 0 {
		return 0, ErrReaderIsClosed
	}

	if len(p) <= 0 {
		return 0, nil
	}

	// 读取数据。
	n, err := m.segments.Read(p)

	// 读不出数据，且写入流已经关闭了，则返回 io.EOF。
	if n <= 0 && atomic.LoadInt32(&m.writerCloseFlag) > 0 {
		// 再次读取数据。
		n, err = m.segments.Read(p)
		if n <= 0 && atomic.LoadInt32(&m.writerCloseFlag) > 0 {
			return 0, io.EOF
		}
	}

	return n, err
}

func (r *readCloser) Close() error {
	return r.closeReader()
}

func (r *writeCloser) Close() error {
	return r.closeWriter()
}

func (r *writeCloser) CloseByError(_ error) error {
	return r.closeWriter()
}

// 关闭写入。
func (m *writerAtToReader) closeWriter() error {
	if atomic.CompareAndSwapInt32(&m.writerCloseFlag, 0, 1) {
		return nil
	}
	return ErrWriterIsClosed
}

// 关闭读取。
func (m *writerAtToReader) closeReader() error {
	if atomic.CompareAndSwapInt32(&m.readerCloseFlag, 0, 1) {
		m.segments.Discard()
		return nil
	}
	return ErrReaderIsClosed
}
