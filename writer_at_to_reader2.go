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

type readCloser2 interface {
	Reader
	closeRead() error
}

type readCloser2Impl struct {
	readCloser2
}

type writerAtToReader2 struct {
	data                             map[int64][]byte
	lock                             sync.Mutex
	readPosition                     int64
	writerCloseFlag, readerCloseFlag int32
}

// NewWriteAtToReader2 同 NewWriteAtToReader 类似，但是 wc 会持有写入的字节，而不是复制一份。然后从 rc 处读出该字节。
func NewWriteAtToReader2() (wc WriteAtCloser, rc ReadCloser) {
	m := &writerAtToReader2{}
	return &writeAtCloserImpl{m}, &readCloser2Impl{m}
}

// ReadAll 读取出所有字节，直到 r 返回 io.EOF。
func ReadAll(r Reader) ([]byte, error) {
	data := &bytes.Buffer{}
	for {
		bs, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return data.Bytes(), err
		}
		data.Write(bs)
	}
	return data.Bytes(), nil
}

// WriteAt 写入数据。
func (m *writerAtToReader2) WriteAt(bs []byte, offset int64) (writtenLength int, err error) {
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

	m.lock.Lock()
	defer m.lock.Unlock()

	// 忽略掉读取位置以前的字节写入。
	if m.readPosition > offset {
		bs = bs[min(m.readPosition-offset, int64(len(bs))):]
		if len(bs) <= 0 {
			return
		}
		offset = m.readPosition
	}

	// 保存数据。
	if m.data == nil {
		m.data = make(map[int64][]byte)
		m.data[offset] = bs
		return
	}
	m.write(offset, bs)

	return
}

// Read 读取数据。
func (m *writerAtToReader2) Read() ([]byte, error) {
	// 已经关闭了就不能再读取流。
	if atomic.LoadInt32(&m.readerCloseFlag) > 0 {
		return nil, ErrReaderIsClosed
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	// 读取数据。
	bs := m.read()

	// 读不出数据，且写入流已经关闭了，则返回 io.EOF。
	if len(bs) <= 0 && atomic.LoadInt32(&m.writerCloseFlag) > 0 {
		// 再次读取数据。
		bs = m.read()
		if len(bs) <= 0 && atomic.LoadInt32(&m.writerCloseFlag) > 0 {
			return nil, io.EOF
		}
	}

	return bs, nil
}

// Close 关闭写入流。
func (c *readCloser2Impl) Close() error { return c.closeRead() }

func (m *writerAtToReader2) read() []byte {
	if len(m.data) <= 0 {
		return nil
	}

	offsets := slices.SortedFunc(maps.Keys(m.data), func(x, y int64) int { return int(x - y) })
	if offsets[0] > m.readPosition {
		return nil
	}

	for _, v := range offsets {
		data := m.data[v]
		dataLength := int64(len(data))
		if dataLength <= 0 || dataLength+v <= m.readPosition {
			delete(m.data, v)
			continue
		}

		data = data[m.readPosition-v:]
		delete(m.data, v)
		m.readPosition += int64(len(data))
		return data
	}

	return nil
}

func (m *writerAtToReader2) write(offset int64, bs []byte) {
	if len(m.data) <= 0 {
		m.data[offset] = bs
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
			m.data[offset] = bs
			return
		}

		// end 小于下一个元素右边，则添加新元素，并更新下一个元素的长度。
		if m.end(nextOffset) > end {
			m.data[offset] = bs
			nextData := m.data[nextOffset]
			delete(m.data, nextOffset)
			m.data[end] = nextData[end-nextOffset:]
			return
		}

		// 删除覆盖了的元素，添加新元素。
		coverIndex := m.findCoverIndex(end, offsets, index+2)
		for i := index + 1; i <= coverIndex; i++ {
			delete(m.data, offsets[i])
		}
		m.data[offset] = bs

		// 覆盖了下一个元素，则更新下一个元素的长度。
		if coverIndex < len(offsets)-1 && end > offsets[coverIndex+1] {
			nextOffset = offsets[coverIndex+1]
			nextData := m.data[nextOffset]
			delete(m.data, nextOffset)
			m.data[end] = nextData[end-nextOffset:]
		}
		return
	}

	// offset 是最后一个元素。
	if index == len(offsets) {
		nextOffset := offsets[len(offsets)-1]
		nextData := m.data[nextOffset]
		nextEnd := m.end(nextOffset)

		// offset 在最后一个元素内部。
		if nextEnd > offset {
			// end 也在最后一个元素内部，则添加新元素，并更新下一个元素的长度。
			if end < nextEnd {
				m.data[offset] = bs
				m.data[nextOffset] = nextData[:offset-nextOffset]
				m.data[end] = nextData[end-nextOffset:]
				return
			}

			// end 不小于最后一个元素的 end，则添加新元素，并更新下一个元素的长度。
			m.data[offset] = bs
			m.data[nextOffset] = nextData[:offset-nextOffset]
			return
		}

		// offset 不小于下一个元素的 end，则直接添加新元素。
		m.data[offset] = bs
		return
	}

	// offset 在中间位置。

	// 存在相同 offset 的元素。
	currentOffset := offsets[index]
	if currentOffset == offset {
		// 小于当前元素的右边，则添加新元素，并更新当前元素长度。
		if m.end(currentOffset) > end {
			nextData := m.data[currentOffset]
			m.data[currentOffset] = bs
			m.data[end] = nextData[end-currentOffset:]
			return
		}

		// 删除覆盖了的元素，添加新元素。
		coverIndex := m.findCoverIndex(end, offsets, index)
		for i := index; i <= coverIndex; i++ {
			delete(m.data, offsets[i])
		}
		m.data[currentOffset] = bs

		// 如果覆盖到了下一个元素，则更新下一个元素长度。
		if coverIndex < len(offsets)-1 && end > offsets[coverIndex+1] {
			nextOffset := offsets[coverIndex+1]
			nextData := m.data[nextOffset]
			delete(m.data, nextOffset)
			m.data[end] = nextData[end-nextOffset:]
		}
		return
	}

	// 不存在相同 offset 元素。

	// end 小于前一个元素的右边，则添加新元素，并更新前一个元素的长度。
	prevOffset := offsets[index-1]
	prevEnd := m.end(prevOffset)
	prevData := m.data[prevOffset]
	if end < prevEnd {
		m.data[offset] = bs
		m.data[end] = prevData[end-prevOffset:]
		m.data[prevOffset] = prevData[:offset-prevOffset]
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
		delete(m.data, offsets[i])
	}
	m.data[offset] = bs

	// offset 小于前一个元素的右边，则更新前一个元素的长度。
	if !exceed {
		m.data[prevOffset] = prevData[:offset-prevOffset]
	}

	// 覆盖到了下一个元素，则更新下一个元素的长度。
	if coverIndex < len(offsets)-1 && end > offsets[coverIndex+1] {
		nextOffset := offsets[coverIndex+1]
		nextData := m.data[nextOffset]
		delete(m.data, nextOffset)
		m.data[end] = nextData[end-nextOffset:]
	}
}

// 一个元素的末尾值。
func (m *writerAtToReader2) end(offset int64) int64 {
	bs := m.data[offset]
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
	offsets = slices.SortedFunc(maps.Keys(m.data), func(x, y int64) int { return int(x - y) })

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

func (m *writerAtToReader2) closeWrite() error {
	if atomic.CompareAndSwapInt32(&m.writerCloseFlag, 0, 1) {
		return nil
	}
	return ErrWriterIsClosed
}

func (m *writerAtToReader2) closeRead() error {
	if atomic.CompareAndSwapInt32(&m.readerCloseFlag, 0, 1) {
		m.data = nil
		return nil
	}
	return ErrReaderIsClosed
}
