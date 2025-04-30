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

import "sync"

// SegmentLength 一个段占用的字节数。
const SegmentLength = 1000

// 缓冲段。
var segmentPool = sync.Pool{New: func() any { return &segment{} }}

// SegmentManager 多个段管理。
type SegmentManager struct {
	segments     map[int]*segment
	readPosition int
	lock         sync.Mutex
	axisMarker   AxisMarker
}

// 段数据。
type segment struct {
	offset int
	data   [SegmentLength]byte
}

func (m *SegmentManager) WriteAt(bs []byte, offset int64) (int, error) {
	length := len(bs)

	m.lock.Lock()
	defer m.lock.Unlock()

	// 丢弃在读取位置前的写入。
	if offset < int64(m.readPosition) {
		bs = bs[min(int64(len(bs)), int64(m.readPosition)-offset):]
		offset = int64(m.readPosition)
	}
	if len(bs) <= 0 {
		return length, nil
	}

	// 获取写入的数据应分配到哪些序号的段里面。
	start := int(offset / SegmentLength)
	n := offset + int64(len(bs))
	end := int(n / SegmentLength)
	if n%SegmentLength > 0 {
		end++
	}

	// 写入数据。
	if m.segments == nil {
		m.segments = make(map[int]*segment)
	}
	for i := start; i < end; i++ {
		sg, ok := m.segments[i]
		if !ok {
			m.segments[i] = allocateSegment()
			sg = m.segments[i]
		}
		if i == start {
			writeOffset := offset % SegmentLength
			writeLength := min(int64(len(bs)), SegmentLength-writeOffset)
			copy(sg.data[writeOffset:], bs[:writeLength])
			bs = bs[writeLength:]
		} else {
			writeLength := min(len(bs), SegmentLength)
			copy(sg.data[:], bs[:writeLength])
			bs = bs[writeLength:]
		}
	}

	// 记录写入位置。
	m.axisMarker.Mark(int(offset), length)

	return length, nil
}

func (m *SegmentManager) Read(bs []byte) (int, error) {
	// 没有段可读就返回函数。
	if len(bs) <= 0 {
		return 0, nil
	}
	if len(m.segments) <= 0 {
		return 0, nil
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	// 数据还没写入，返回函数。
	getLen := m.axisMarker.Get(m.readPosition, len(bs))
	if getLen <= 0 {
		return 0, nil
	}

	// 获取要读取数据的段。
	index := m.readPosition / SegmentLength
	sg := m.segments[index]

	// 读取数据到字节数组 bs。
	off := m.readPosition % SegmentLength
	canReadLength := int(min(int64(getLen), int64(SegmentLength-off)))
	copy(bs, sg.data[off:off+canReadLength])

	// 读取完了，回收段。
	if off+canReadLength == SegmentLength {
		recycleSegment(sg)
		delete(m.segments, index)
	}

	// 更新读取下标。
	m.readPosition += canReadLength

	return canReadLength, nil
}

// Discard 丢弃所有写入的数据，重置读取光标。
func (m *SegmentManager) Discard() {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, v := range m.segments {
		recycleSegment(v)
	}
	m.segments = nil
	m.readPosition = 0
}

// 分配段。
func allocateSegment() *segment {
	sg := segmentPool.Get().(*segment)
	return sg
}

// 回收段。
func recycleSegment(sg *segment) {
	if sg == nil {
		return
	}
	segmentPool.Put(sg)
}
