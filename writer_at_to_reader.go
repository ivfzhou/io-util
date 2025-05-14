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

// WriteAtCloser 数据写入接口。
type WriteAtCloser interface {
	io.WriterAt
	io.Closer

	// CloseByError 关闭写入。
	//
	// Deprecated: 使用 Close 代替。
	CloseByError(error) error
}

type writeAtCloser interface {
	io.WriterAt
	closeWrite() error
}

type readCloser interface {
	io.Reader
	closeRead() error
}

type writerAtToReader struct {
	segments                         SegmentManager
	lock                             sync.Mutex
	writerCloseFlag, readerCloseFlag int32
}

type writeAtCloserImpl struct {
	writeAtCloser
}

type readCloserImpl struct {
	readCloser
}

// NewWriteAtToReader 获取一个 WriteAtCloser 和 io.ReadCloser 对象，其中 wc 用于并发写入数据，与此同时 rc 读取出已经写入好的数据。
//
// wc：写入流。写入完毕后关闭，则 rc 会全部读取完后返回 io.EOF。
//
// rc：读取流。
//
// wc 发生的错误会传递给 rc 返回。
func NewWriteAtToReader() (wc WriteAtCloser, rc io.ReadCloser) {
	w := &writerAtToReader{}
	return &writeAtCloserImpl{w}, &readCloserImpl{w}
}

// NewWriteAtReader 获取一个 WriteAtCloser 和 io.ReadCloser 对象，其中 wc 用于并发写入数据，与此同时 rc 读取出已经写入好的数据。
//
// wc：写入流。写入完毕后关闭，则 rc 会全部读取完后返回 io.EOF。
//
// rc：读取流。
//
// wc 发生的错误会传递给 rc 返回。
//
// Deprecated: 使用 NewWriteAtToReader 代替。
func NewWriteAtReader() (wc WriteAtCloser, rc io.ReadCloser) {
	return NewWriteAtToReader()
}

// WriteAt 写入数据。
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

// Read 读取数据。
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

// Close 关闭写入流。
func (c *writeAtCloserImpl) Close() error { return c.closeWrite() }

// CloseByError 关闭写入流。
func (c *writeAtCloserImpl) CloseByError(error) error { return c.closeWrite() }

// Close 关闭读取流。
func (c *readCloserImpl) Close() error { return c.closeRead() }

func (m *writerAtToReader) closeWrite() error {
	if atomic.CompareAndSwapInt32(&m.writerCloseFlag, 0, 1) {
		return nil
	}
	return ErrWriterIsClosed
}

func (m *writerAtToReader) closeRead() error {
	if atomic.CompareAndSwapInt32(&m.readerCloseFlag, 0, 1) {
		m.segments.Discard()
		return nil
	}
	return ErrReaderIsClosed
}
