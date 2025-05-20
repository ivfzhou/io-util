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
	"bytes"
	"errors"
	"io"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
)

// ReadCloser 数据读取接口。
type ReadCloser interface {
	Read() ([]byte, error)
	io.Closer
}

// Reader 数据读取接口。
type Reader interface {
	Read() ([]byte, error)
}

type writerAtToReader2 struct {
	// 存放写入的临时字节数据。
	tmpData map[int64][]byte
	// 读与写互斥。
	lock sync.Mutex
	// 读取位置。
	readPosition int64
	// 关闭流标识。
	writerCloseFlag, readerCloseFlag int32
	// 发生的错误信息。
	err AtomicError
}

type readCloser2 struct {
	*writerAtToReader2
}

type writeCloser2 struct {
	*writerAtToReader2
}

type changeReader struct {
	// 被包装的流。
	rc ReadCloser
	// 临时字节数据。
	tmp []byte
	// 标识流是否已读完。
	eof bool
}

// NewWriteAtToReader2 同 NewWriteAtToReader 类似，但是 wc 会持有写入的字节，而不是复制一份。然后从 rc 处读出这些字节。
//
// 注意：对 wc 写入的字节变量的底层字节数组的修改，可能会影响到 rc 读取的字节也跟着被修改。因为它们是同一个底层字节数组。
func NewWriteAtToReader2() (wc WriteAtCloser, rc ReadCloser) {
	m := &writerAtToReader2{}
	return &writeCloser2{m}, &readCloser2{m}
}

// ToReader 转换成 io.ReadCloser 对象。
func ToReader(r ReadCloser) io.ReadCloser {
	return &changeReader{rc: r}
}

// ReadAll 读取出所有字节，直到 r 返回 io.EOF。如果全部读出，没有错误，则 error 返回 nil，EOF 被剔除。
//
// 注意：如果 r 是空，将触发恐慌。
func ReadAll(r Reader) ([]byte, error) {
	data := &bytes.Buffer{}
	// 循环读取。
	for {
		bs, err := r.Read()
		// 写入读出的字节。
		if len(bs) > 0 {
			data.Write(bs)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		// 发生错误便返回。
		if err != nil {
			return data.Bytes(), err
		}
	}
	return data.Bytes(), nil
}

// WriteAt 写入数据。
func (m *writerAtToReader2) WriteAt(bs []byte, offset int64) (written int, err error) {
	// 与其他读写线程互斥。
	m.lock.Lock()
	defer m.lock.Unlock()

	// 已经发生了错误，就不再写入了。
	if m.err.HasSet() {
		return 0, m.err.Get()
	}

	// 写入流已经关闭了，就不允许再写入。
	if atomic.LoadInt32(&m.writerCloseFlag) > 0 {
		return 0, ErrWriterIsClosed
	}

	// 写入位置不可为负数。
	if offset < 0 {
		return 0, ErrOffsetCannotNegative
	}

	// 忽略写入的字节数为空。
	if len(bs) <= 0 {
		return
	}

	// 已经关闭了读取流，则不必再关心写入数据。
	written = len(bs)
	if atomic.LoadInt32(&m.readerCloseFlag) > 0 {
		return
	}

	// 丢弃掉读取位置以前的字节写入。
	if m.readPosition > offset {
		bs = bs[min(m.readPosition-offset, int64(len(bs))):]
		if len(bs) <= 0 {
			return
		}
		offset = m.readPosition
	}

	// 保存字节数据。
	if m.tmpData == nil {
		m.tmpData = make(map[int64][]byte)
		m.tmpData[offset] = bs
		return
	}
	m.write(offset, bs)

	return
}

// Read 读取数据。
func (m *writerAtToReader2) Read() ([]byte, error) {
	// 与其他读写线程互斥。
	m.lock.Lock()
	defer m.lock.Unlock()

	// 已经设置了错误，就不必再读取了。
	if m.err.HasSet() {
		return nil, m.err.Get()
	}

	// 已经关闭了读取流，就不能再读取。
	if atomic.LoadInt32(&m.readerCloseFlag) > 0 {
		return nil, ErrReaderIsClosed
	}

	// 读取数据。
	bs := m.read()

	// 读不出数据，且写入流已经关闭了，则返回 io.EOF。
	if len(bs) <= 0 && atomic.LoadInt32(&m.writerCloseFlag) > 0 {
		if err := m.err.Get(); err != nil { // 当刚好设置了错误，而上面没有拦截到时，应当返回错误。
			return nil, err
		}
		return nil, io.EOF
	}

	return bs, nil
}

// Close 关闭写入流。
func (c *readCloser2) Close() error { return c.closeRead() }

// CloseByError 关闭写入流。
func (c *writeCloser2) CloseByError(err error) error {
	return c.closeWrite(err)
}

// Close 关闭读取流。
func (c *writeCloser2) Close() error {
	return c.closeWrite(nil)
}

// Read 实现 io.Reader 接口。
func (c *changeReader) Read(p []byte) (n int, err error) {
	// 临时字节已读取完毕。
	if len(c.tmp) <= 0 {
		c.tmp, err = c.rc.Read()
		if errors.Is(err, io.EOF) { // 读取完毕了。
			c.eof = true
			if len(c.tmp) <= 0 { // 没有多余字节需要处理，直接返回 EOF。
				return 0, io.EOF
			}
		}

		// 发生了错误，直接返回该错误。
		if err != nil {
			// 将读取到的字节复制出去。
			n = copy(p, c.tmp)
			if c.tmp != nil {
				c.tmp = c.tmp[:n]
			}
			if len(c.tmp) <= 0 {
				c.tmp = nil // 释放内存。
			}
			return n, err
		}

		// 没有读取到字节数据。
		if len(c.tmp) <= 0 {
			c.tmp = nil
			return 0, nil
		}
	}

	// 复制字节数据。
	n = copy(p, c.tmp)
	c.tmp = c.tmp[n:]

	// 释放内存。
	if len(c.tmp) <= 0 {
		if c.eof { // 全部读完了。
			return n, io.EOF
		}
		c.tmp = nil
	}

	return n, nil
}

// Close 实现 io.Closer 接口。
func (c *changeReader) Close() error { return c.rc.Close() }

func (m *writerAtToReader2) read() []byte {
	// 没有字节数据读取，直接返回。
	if len(m.tmpData) <= 0 {
		return nil
	}

	// 将各块字节数据排序，寻找符合读取位置的字节数据块。
	offsets := slices.SortedFunc(maps.Keys(m.tmpData), func(x, y int64) int { return int(x - y) })
	if offsets[0] > m.readPosition {
		return nil // 还没有写入，直接返回。
	}

	// 将字节数据块取出。
	for _, v := range offsets {
		data := m.tmpData[v]
		dataLength := int64(len(data))
		if dataLength <= 0 || dataLength+v <= m.readPosition { // 该字节数据块在读取位置之前，可丢弃。
			delete(m.tmpData, v)
			continue
		}

		if v > m.readPosition {
			return nil // 还没有写入，直接返回。
		}

		// 截取符合读取位置的字节数据。
		data = data[m.readPosition-v:]
		delete(m.tmpData, v)
		if len(data) <= 0 { // 没有数据，再循环寻找。
			continue
		}

		// 更新读取位置。
		m.readPosition += int64(len(data))

		return data
	}

	return nil // 数据都不符合要求，返回空。
}

func (m *writerAtToReader2) write(offset int64, bs []byte) {
	// 还没有元素，就直接写入。
	if len(m.tmpData) <= 0 {
		m.tmpData[offset] = bs
		return
	}

	// 找出 offset 所在位置。
	index, offsets := m.findIndex(offset)

	// offset 在最前面。
	end := offset + int64(len(bs))
	if index < 0 {
		// end 不超过下一个元素的左边，则添加新元素。
		nextOffset := offsets[index+1]
		if end <= nextOffset {
			m.tmpData[offset] = bs
			return
		}

		// end 小于下一个元素右边，则添加新元素，并更新下一个元素的长度。
		if m.end(nextOffset) > end {
			m.tmpData[offset] = bs
			nextData := m.tmpData[nextOffset]
			delete(m.tmpData, nextOffset)
			m.tmpData[end] = nextData[end-nextOffset:]
			return
		}

		// 删除覆盖了的元素，添加新元素。
		coverIndex := m.findCoverIndex(end, offsets, index+2)
		for i := index + 1; i <= coverIndex; i++ {
			delete(m.tmpData, offsets[i])
		}
		m.tmpData[offset] = bs

		// 覆盖了下一个元素，则更新下一个元素的长度。
		if coverIndex < len(offsets)-1 && end > offsets[coverIndex+1] {
			nextOffset = offsets[coverIndex+1]
			nextData := m.tmpData[nextOffset]
			delete(m.tmpData, nextOffset)
			m.tmpData[end] = nextData[end-nextOffset:]
		}
		return
	}

	// offset 是最后一个元素。
	if index == len(offsets) {
		nextOffset := offsets[len(offsets)-1]
		nextData := m.tmpData[nextOffset]
		nextEnd := m.end(nextOffset)

		// offset 在最后一个元素内部。
		if nextEnd > offset {
			// end 也在最后一个元素内部，则添加新元素，并更新下一个元素的长度。
			if end < nextEnd {
				m.tmpData[offset] = bs
				m.tmpData[nextOffset] = nextData[:offset-nextOffset]
				m.tmpData[end] = nextData[end-nextOffset:]
				return
			}

			// end 不小于最后一个元素的 end，则添加新元素，并更新下一个元素的长度。
			m.tmpData[offset] = bs
			m.tmpData[nextOffset] = nextData[:offset-nextOffset]
			return
		}

		// offset 不小于下一个元素的 end，则直接添加新元素。
		m.tmpData[offset] = bs
		return
	}

	// offset 在中间位置。

	// 存在相同 offset 的元素。
	currentOffset := offsets[index]
	if currentOffset == offset {
		// 小于当前元素的右边，则添加新元素，并更新当前元素长度。
		if m.end(currentOffset) > end {
			nextData := m.tmpData[currentOffset]
			m.tmpData[currentOffset] = bs
			m.tmpData[end] = nextData[end-currentOffset:]
			return
		}

		// 删除覆盖了的元素，添加新元素。
		coverIndex := m.findCoverIndex(end, offsets, index)
		for i := index; i <= coverIndex; i++ {
			delete(m.tmpData, offsets[i])
		}
		m.tmpData[currentOffset] = bs

		// 如果覆盖到了下一个元素，则更新下一个元素长度。
		if coverIndex < len(offsets)-1 && end > offsets[coverIndex+1] {
			nextOffset := offsets[coverIndex+1]
			nextData := m.tmpData[nextOffset]
			delete(m.tmpData, nextOffset)
			m.tmpData[end] = nextData[end-nextOffset:]
		}
		return
	}

	// 不存在相同 offset 元素。

	// end 小于前一个元素的右边，则添加新元素，并更新前一个元素的长度。
	prevOffset := offsets[index-1]
	prevEnd := m.end(prevOffset)
	prevData := m.tmpData[prevOffset]
	if end < prevEnd {
		m.tmpData[offset] = bs
		m.tmpData[end] = prevData[end-prevOffset:]
		m.tmpData[prevOffset] = prevData[:offset-prevOffset]
		return
	}

	// offset 不小于了前一个元素的右边。
	exceed := prevEnd <= offset

	// 删除覆盖了的元素，添加新元素。
	coverIndex := m.findCoverIndex(end, offsets, index)
	i := index - 1
	if exceed {
		i = index
	}
	for ; i <= coverIndex; i++ {
		delete(m.tmpData, offsets[i])
	}
	m.tmpData[offset] = bs

	// offset 小于前一个元素的右边，则更新前一个元素的长度。
	if !exceed {
		m.tmpData[prevOffset] = prevData[:offset-prevOffset]
	}

	// 覆盖到了下一个元素，则更新下一个元素的长度。
	if coverIndex < len(offsets)-1 && end > offsets[coverIndex+1] {
		nextOffset := offsets[coverIndex+1]
		nextData := m.tmpData[nextOffset]
		delete(m.tmpData, nextOffset)
		m.tmpData[end] = nextData[end-nextOffset:]
	}
}

// 一个元素的末尾值。
func (m *writerAtToReader2) end(offset int64) int64 {
	bs := m.tmpData[offset]
	return int64(len(bs)) + offset
}

// 找出 end 可以覆盖到哪个下标。
func (m *writerAtToReader2) findCoverIndex(end int64, offsets []int64, startIndex int) int {
	lastIndex := startIndex
	numOfOffset := len(offsets)
	for lastIndex < numOfOffset && end >= m.end(offsets[lastIndex]) {
		lastIndex++
	}
	return lastIndex - 1
}

// 找出 offset 可以添加到哪个下标下。
func (m *writerAtToReader2) findIndex(offset int64) (index int, offsets []int64) {
	// 先给各数据块排序。
	offsets = slices.SortedFunc(maps.Keys(m.tmpData), func(x, y int64) int { return int(x - y) })

	// 没数据，返回 -1 表示添加到最前面。
	if len(offsets) <= 0 {
		index = -1
		return
	}

	// offset 比最大值还大，就添加到最后面。
	maxIndex := len(offsets) - 1
	if offsets[maxIndex] < offset {
		index = maxIndex + 1
		return
	}

	// offset 比最小值还小，就添加到最前面。
	minIndex := 0
	if offsets[minIndex] > offset {
		index = -1
		return
	}

	// 二分查找。
	middleIndex := (maxIndex + minIndex) / 2
	for {
		l := offsets[middleIndex]
		if l == offset { // 存在相等的 offset，添加到这个下标。
			index = middleIndex
			return
		}

		// 更新下标。
		if l < offset {
			minIndex = middleIndex
		} else {
			maxIndex = middleIndex
		}
		middleIndex = (minIndex + maxIndex) / 2

		// 下标已不再更新。
		if minIndex == middleIndex {
			if offsets[minIndex] == offset { // offset 等于左下标的。
				index = minIndex
				return
			} else if offsets[maxIndex] == offset { // offset 等于右下标的。
				index = maxIndex
				return
			}
			// offset 在中间位置。
			index = middleIndex + 1
			return
		}
	}
}

func (m *writerAtToReader2) closeWrite(err error) error {
	if err != nil { // 先设置错误，避免错误信息丢失。
		m.err.Set(err)
	}
	if atomic.CompareAndSwapInt32(&m.writerCloseFlag, 0, 1) {
		return nil
	}
	return ErrWriterIsClosed
}

func (m *writerAtToReader2) closeRead() error {
	if atomic.CompareAndSwapInt32(&m.readerCloseFlag, 0, 1) {
		m.tmpData = nil
		return nil
	}
	return ErrReaderIsClosed
}
