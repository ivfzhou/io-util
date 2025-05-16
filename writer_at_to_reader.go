package io_util

import (
	"errors"
	"io"
	"os"
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
	// error 将传递给 Reader 感知。
	CloseByError(error) error
}

type writerAtToReader struct {
	// 字节临时存放于外存。
	tmpFile *os.File
	// 读和写字节互斥。
	lock sync.Mutex
	// 记录写入的字节的位置。
	axisMarker AxisMarker
	// 读取位置。
	readPosition int64
	// 流关闭标识。
	writerCloseFlag, readerCloseFlag int32
	// 保存错误信息。
	err AtomicError
}

type writeAtCloser struct {
	*writerAtToReader
}

type readCloser struct {
	*writerAtToReader
}

// NewWriteAtToReader 获取一个 WriteAtCloser 和 io.ReadCloser 对象，其中 wc 用于并发写入数据，与此同时 rc 读取出已经写入好的数据。
//
// wc：写入流。字节临时写入到磁盘。写入完毕后关闭，则 rc 会全部读取完后返回 io.EOF。
//
// rc：读取流。
//
// wc 发生的错误会传递给 rc 返回。
func NewWriteAtToReader() (wc WriteAtCloser, rc io.ReadCloser) {
	temp, err := os.CreateTemp("", "ivfzhou_WriteAtToReader_*.tmp")
	w := &writerAtToReader{tmpFile: temp}
	if err != nil {
		// 创建临时文件失败，记录错误信息。
		w.err.Set(err)
	}
	return &writeAtCloser{w}, &readCloser{w}
}

// NewWriteAtReader 同 NewWriteAtToReader。
//
// Deprecated: 使用 NewWriteAtToReader 代替。
func NewWriteAtReader() (wc WriteAtCloser, rc io.ReadCloser) {
	return NewWriteAtToReader()
}

// WriteAt 写入数据。
func (m *writerAtToReader) WriteAt(bs []byte, offset int64) (writtenLength int, err error) {
	// 与其他读写线程互斥。
	m.lock.Lock()
	defer m.lock.Unlock()

	// 已经设置了错误信息就不再工作了。
	if m.err.HasSet() {
		return 0, m.err.Get()
	}

	// 已经关闭了就不允许再写入字节。
	if atomic.LoadInt32(&m.writerCloseFlag) > 0 {
		return 0, ErrWriterIsClosed
	}

	// 写入位置不能为负数。
	if offset < 0 {
		return 0, ErrOffsetCannotNegative
	}

	// 没有写入字节可以忽略。
	if len(bs) <= 0 {
		return
	}

	// 已经关闭了读取流，则不必再关心写入数据。
	writtenLength = len(bs)
	if atomic.LoadInt32(&m.readerCloseFlag) > 0 {
		return
	}

	// 将字节数据保存到临时文件。
	n, err := WriteAtAll(m.tmpFile, offset, bs)

	// 记录写入字节的位置信息。
	writtenLength = int(n)
	m.axisMarker.Mark(int(offset), writtenLength)

	// 记录写入字节到文件发生的错误。
	if err != nil {
		m.err.Set(err)
	}

	return writtenLength, err
}

// Read 读取数据。
func (m *writerAtToReader) Read(p []byte) (int, error) {
	// 与其他读写线程互斥。
	m.lock.Lock()
	defer m.lock.Unlock()

	// 已经发生了错误，就不在工作了。
	if m.err.HasSet() {
		return 0, m.err.Get()
	}

	// 已经关闭了就不能再读取了。
	if atomic.LoadInt32(&m.readerCloseFlag) > 0 {
		return 0, ErrReaderIsClosed
	}

	// 要将字节读取到的字节数组为空，可以忽略。
	if len(p) <= 0 {
		return 0, nil
	}

	// 获取已写入的字节的位置信息。
	getLen := m.axisMarker.Get(int(m.readPosition), len(p))
	if getLen <= 0 {
		// 没有可读取的字节，且写入流已经关闭了，则返回 EOF。
		if atomic.LoadInt32(&m.writerCloseFlag) > 0 {
			return 0, io.EOF
		}
		return 0, nil
	}

	// 读取数据到字节数组。
	n, err := m.tmpFile.ReadAt(p[:getLen], m.readPosition)

	// 更新读取位置。
	m.readPosition += int64(n)

	// 记录读取时发生的错误。
	if err != nil {
		m.err.Set(err)
	}

	return n, err
}

// Close 关闭写入流。
func (c *writeAtCloser) Close() error { return c.closeWrite(nil) }

// CloseByError 关闭写入流。
func (c *writeAtCloser) CloseByError(err error) error {
	return c.closeWrite(err)
}

// Close 关闭读取流。
func (c *readCloser) Close() error { return c.closeRead() }

func (m *writerAtToReader) closeWrite(err error) error {
	if atomic.CompareAndSwapInt32(&m.writerCloseFlag, 0, 1) {
		// 记录错误信息。
		if err != nil {
			m.err.Set(err)
		}
		return nil
	}
	return ErrWriterIsClosed
}

func (m *writerAtToReader) closeRead() error {
	// 清理临时文件，并将发生的错误返回。
	if atomic.CompareAndSwapInt32(&m.readerCloseFlag, 0, 1) {
		var e error
		err := m.tmpFile.Truncate(0)
		if err != nil {
			e = err
		}
		if err = m.tmpFile.Close(); err != nil && e == nil {
			e = err
		}
		if err = os.RemoveAll(m.tmpFile.Name()); err != nil && e == nil {
			e = err
		}
		return e
	}
	return ErrReaderIsClosed
}
