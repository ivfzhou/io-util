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
	"io"
	"sync"
	"sync/atomic"
)

type writerAtToReader3 struct {
	// 保存字节数据。
	segments SegmentManager
	// 关闭标识。
	writerCloseFlag, readerCloseFlag int32
	// 保存错误信息。
	err AtomicError
	// 读写协程互斥。
	lock sync.Mutex
}

type writeAtCloser3 struct {
	*writerAtToReader3
}

type readCloser3 struct {
	*writerAtToReader3
}

// NewWriteAtToReader3 同 NewWriteAtToReader 类似，但是 wc 写入的字节将放置在内存中。
func NewWriteAtToReader3() (wc WriteAtCloser, rc io.ReadCloser) {
	w := &writerAtToReader3{}
	return &writeAtCloser3{w}, &readCloser3{w}
}

// WriteAt 写入数据。
func (m *writerAtToReader3) WriteAt(bs []byte, offset int64) (writtenLength int, err error) {
	// 读与写线程互斥。
	m.lock.Lock()
	defer m.lock.Unlock()

	// 已经发生了错误，就不再读取了。
	if m.err.HasSet() {
		return 0, m.err.Get()
	}

	// 关闭了不可再写入。
	if atomic.LoadInt32(&m.writerCloseFlag) > 0 {
		return 0, ErrWriterIsClosed
	}

	// 写入位置不可为负数。
	if offset < 0 {
		return 0, ErrOffsetCannotNegative
	}

	// 没有字节写入，则可忽略。
	if len(bs) <= 0 {
		return
	}

	// 已经关闭了读取流，则不必再关心写入数据。
	writtenLength = len(bs)
	if atomic.LoadInt32(&m.readerCloseFlag) > 0 {
		return
	}

	// 将字节数据保存。
	writtenLength, err = m.segments.WriteAt(bs, offset)
	if err != nil {
		m.err.Set(err)
	}

	return writtenLength, err
}

// Read 读取数据。
func (m *writerAtToReader3) Read(p []byte) (int, error) {
	// 读与写线程互斥。
	m.lock.Lock()
	defer m.lock.Unlock()

	// 发生了错误就不必再读取了。
	if m.err.HasSet() {
		return 0, m.err.Get()
	}

	// 已经关闭了就不能再读取流。
	if atomic.LoadInt32(&m.readerCloseFlag) > 0 {
		return 0, ErrReaderIsClosed
	}

	// 没有数据，则可忽略。
	if len(p) <= 0 {
		return 0, nil
	}

	// 读取数据。
	n, err := m.segments.Read(p)
	if err != nil {
		m.err.Set(err)
		return n, err
	}

	// 读不出数据，且写入流已经关闭了，则返回 io.EOF。
	if n <= 0 && atomic.LoadInt32(&m.writerCloseFlag) > 0 {
		if err = m.err.Get(); err != nil { // 当刚好设置了错误，而上面没有拦截到时，应当返回错误。
			return 0, err
		}
		return 0, io.EOF
	}

	return n, err
}

// Close 关闭写入流。
func (c *writeAtCloser3) Close() error { return c.closeWrite(nil) }

// CloseByError 关闭写入流。
func (c *writeAtCloser3) CloseByError(err error) error {
	return c.closeWrite(err)
}

// Close 关闭读取流。
func (c *readCloser3) Close() error { return c.closeRead() }

func (m *writerAtToReader3) closeWrite(err error) error {
	if err != nil { // 先设置错误，避免错误信息丢失。
		m.err.Set(err)
	}
	if atomic.CompareAndSwapInt32(&m.writerCloseFlag, 0, 1) {
		return nil
	}
	return ErrWriterIsClosed
}

func (m *writerAtToReader3) closeRead() error {
	// 加锁，避免有数据在写入。
	m.lock.Lock()
	defer m.lock.Unlock()

	if atomic.CompareAndSwapInt32(&m.readerCloseFlag, 0, 1) {
		m.segments.Discard()
		return nil
	}
	return ErrReaderIsClosed
}
