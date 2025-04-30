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
	"fmt"
	"math"
	"strings"
	"unsafe"
)

// AxisMarker 坐标轴标记器。
type AxisMarker struct {
	markedLines []*line
}

// 线段。
type line struct {
	offset, length int
}

// Mark 标记坐标轴，从 offset 位置开始，往右数 length 长度的点。length 为负数表示向左标记点。
//
// 不可并发调用。
func (m *AxisMarker) Mark(offset, length int) {
	// 长度是零，没有标记，可忽略。
	if length == 0 {
		return
	}

	// 长度是负数，则更正成正数。
	if length < 0 {
		additionOverflow(offset, length)
		offset += length
		length = -length
	}

	additionOverflow(offset, length)

	// 获取新元素 offset 可插入的位置。
	index := m.findIndex(offset)

	// 新元素插到最前面。
	newElem := &line{offset: offset, length: length}
	end := newElem.end()
	if index < 0 {
		// 还没有标记的元素，则加入并返回。
		if len(m.markedLines) <= 0 {
			m.append(newElem)
			return
		}

		// 新元素右边不与下一个元素接壤，则向前添加新元素。
		nextIndex := index + 1
		nextElem := m.markedLines[nextIndex]
		if end < nextElem.offset {
			m.prepend(newElem)
			return
		}

		// 新元素右边在下一个元素内，与下一个元素合并。
		if end <= nextElem.end() {
			m.merge(nextElem, newElem)
			return
		}

		// 新元素的右边在下下元素的左边，且不接壤，或者没有下下个元素，则与下一个元素合并。
		if nextIndex == len(m.markedLines)-1 || end < m.markedLines[nextIndex+1].offset {
			m.merge(nextElem, newElem)
			return
		}

		// 合并多个覆盖了的元素。
		m.mergeMore(newElem, index)
		return
	}

	// 新元素插入到最后面。
	if index == len(m.markedLines) {
		prevIndex := index - 1
		prevElem := m.markedLines[prevIndex]

		// 新元素在右边不超过前一个元素右边，则不用添加。
		if end <= prevElem.end() {
			return
		}

		// 新元素左边不超过前一个元素的右边，则合并元素。
		if offset <= prevElem.end() {
			m.merge(prevElem, newElem)
			return
		}

		// 在后面添加新元素。
		m.append(newElem)
		return
	}

	// 是否有存在相同 offset 的元素。
	nextElem := m.markedLines[index]
	if nextElem.offset == offset {
		// 新元素右边不大于当前元素右边，则可忽略添加。
		if nextElem.length >= length {
			return
		}

		// 新元素右边小于下一个元素的左边且不接壤，或者当前元素是最后一个元素，则与当前元素合并。
		if index == len(m.markedLines)-1 || m.markedLines[index+1].offset > end {
			m.merge(nextElem, newElem)
			return
		}

		// 合并多个覆盖了的元素。
		m.mergeMore(newElem, index)
		return
	}

	// 新元素在中间位置。
	prevIndex := index - 1
	prevElem := m.markedLines[prevIndex]

	// 新元素右边不超过上一个元素右边，则可忽略添加。
	if prevElem.end() >= end {
		return
	}

	// 新元素与前元素无交集。
	if offset > prevElem.end() {
		// 新元素的右边小于下一个元素的左边且不接壤，则添加新元素。
		if end < nextElem.offset {
			m.insert(newElem, index)
			return
		}

		// 新元素小于下下一个元素左边且不接壤，或者下一个元素是最后一个元素，则与下一个元素合并。
		if index == len(m.markedLines)-1 || end < m.markedLines[index+1].offset {
			m.merge(nextElem, newElem)
			return
		}

		// 合并多个覆盖了的元素。
		m.mergeMore(newElem, index)
		return
	}

	// 新元素与前元素有交集。

	// 新元素右边小于下一个元素左边且不接壤，则与前一个元素合并。
	if end < nextElem.offset {
		m.merge(prevElem, newElem)
	}

	// 合并多个覆盖了的元素。
	m.mergeMore(newElem, index-1)

	// 空位大多，缩容。
	m.shrinkCapacity()
}

// Get 在坐标轴上，从 offset 开始向右获取被连续标记的长度，最大不超过 length。length 为负数表示向左标记。
//
// 不可与 Mark 并发调用。
//
// 返回连续标记的长度。
func (m *AxisMarker) Get(offset, length int) int {
	// 没有要获取的长度可直接返回。
	if length == 0 {
		return 0
	}

	// 长度是负数，则更正成正数。
	if length < 0 {
		additionOverflow(offset, length)
		offset += length
		length = -length
	}

	additionOverflow(offset, length)

	// 找出 offset 在哪个位置。
	index := m.findIndex(offset)

	// 在最前面，说明从 offset 开始没有连续标记。
	if index < 0 {
		return 0
	}

	// 在最后面。
	if index >= len(m.markedLines) {
		// offset 不超过前一个元素的右边。
		prevElem := m.markedLines[index-1]
		if prevElem.end() > offset {
			return min(prevElem.end()-offset, length)
		}
		return 0
	}

	// 判断有没有直接命中的元素。
	if m.markedLines[index].offset == offset {
		return min(m.markedLines[index].length, length)
	}

	// offset 在中间，判断元素长度有没有达到。
	prevElem := m.markedLines[index-1]
	if prevElem.end() > offset {
		return min(prevElem.end()-offset, length)
	}

	return 0
}

// GetMaxMarkLine 获取从某点开始连续被标记的长度。
// Deprecated: 使用 Get 代替。
func (m *AxisMarker) GetMaxMarkLine(begin int64) int64 {
	if len(m.markedLines) <= 0 {
		return 0
	}

	index := m.findIndex(int(begin))
	if index < 0 {
		return 0
	}
	if index >= len(m.markedLines) {
		return 0
	}
	elem := m.markedLines[index]
	if int(begin) >= elem.end() {
		return 0
	}

	return int64(elem.end()) - begin
}

// String 返回坐标轴标记信息。
func (m *AxisMarker) String() string {
	sb := strings.Builder{}
	sb.WriteString("{")
	for i := 0; i < len(m.markedLines)-1; i++ {
		l := m.markedLines[i]
		sb.WriteString(fmt.Sprintf("[%d,%d), ", l.offset, l.offset+l.length))
	}
	if len(m.markedLines) > 0 {
		l := m.markedLines[len(m.markedLines)-1]
		sb.WriteString(fmt.Sprintf("[%d,%d)}", l.offset, l.offset+l.length))
	}
	return sb.String()
}

// 确保 x - y 计算不会溢出。
func substractionOverflow(x, y int) {
	if x < 0 && y > 0 {
		if x < -math.MaxInt+y {
			panic(fmt.Sprintf("calculate overflow: %d - %d", x, y))
		}
		return
	}
	if x > 0 && y < 0 {
		if x > math.MaxInt+y {
			panic(fmt.Sprintf("calculate overflow: %d - %d", x, y))
		}
	}
}

// 确保 x + y 计算不会溢出。
func additionOverflow(x, y int) {
	if x < 0 && y < 0 {
		if x < -(math.MaxInt + y) {
			panic(fmt.Sprintf("calculate overflow: %d + %d", x, y))
		}
		return
	}
	if x > 0 && y > 0 {
		if x > math.MaxInt-y {
			panic(fmt.Sprintf("calculate overflow: %d + %d", x, y))
		}
	}
}

// 增加数组长度。
func (m *AxisMarker) expandCapacity(number int) {
	if cap(m.markedLines)-len(m.markedLines) >= number {
		m.markedLines = unsafe.Slice(&m.markedLines[0], len(m.markedLines)+number)
	} else {
		tmp := make([]*line, len(m.markedLines)+number)
		copy(tmp, m.markedLines)
		m.markedLines = tmp
	}
}

// 找出 offset 应该添加到数组的哪个下标。-1 表示添加到最前面，大于数组长度表示添加到最后面。
func (m *AxisMarker) findIndex(offset int) int {
	// 没数据，返回 -1 表示添加到最前面。
	if len(m.markedLines) <= 0 {
		return -1
	}

	// offset 比最大值还大，就添加到最后面。
	maxIndex := len(m.markedLines) - 1
	if m.markedLines[maxIndex].offset < offset {
		return maxIndex + 1
	}

	// offset 比最小值还小，就添加到最前面。
	minIndex := 0
	if m.markedLines[minIndex].offset > offset {
		return -1
	}

	// 二分查找。
	middleIndex := (maxIndex + minIndex) / 2
	for {
		l := m.markedLines[middleIndex]
		if l.offset == offset { // 存在相等的 offset，添加到这个下标。
			return middleIndex
		}

		// 更新下标。
		if l.offset < offset {
			minIndex = middleIndex
			middleIndex = (minIndex + maxIndex) / 2
		} else {
			maxIndex = middleIndex
			middleIndex = (minIndex + maxIndex) / 2
		}

		// 下标已不再更新。
		if minIndex == middleIndex {
			// offset 等于左下标的。
			if m.markedLines[minIndex].offset == offset {
				return minIndex
			} else if m.markedLines[maxIndex].offset == offset { // offset 等于右下标的。
				return maxIndex
			}
			// offset 在中间位置。
			return middleIndex + 1
		}
	}
}

// 在数组前面添加一个元素。
func (m *AxisMarker) prepend(elem *line) {
	if cap(m.markedLines) > len(m.markedLines) {
		m.expandCapacity(1)
		copy(m.markedLines[1:], m.markedLines)
		m.markedLines[0] = elem
	} else {
		tmp := make([]*line, 0, len(m.markedLines)+1)
		tmp = append(tmp, elem)
		tmp = append(tmp, m.markedLines...)
		m.markedLines = tmp
	}
}

// 在数组后面添加一个元素。
func (m *AxisMarker) append(elem *line) {
	m.markedLines = append(m.markedLines, elem)
}

// 从 index 位置开始，合并后面被 elem 覆盖了的元素。
func (m *AxisMarker) mergeMore(elem *line, index int) {
	var (
		lastIndex   int
		modifyIndex int
	)

	if index < 0 { // 从最左边开始合并。
		lastIndex = index + 1
		modifyIndex = index + 1
	} else if index >= len(m.markedLines) { // 从最右边合并。
		return
	} else {
		lastIndex = index
		modifyIndex = index
	}

	for i := lastIndex + 1; i < len(m.markedLines) && elem.end() >= m.markedLines[i].offset; i++ {
		lastIndex = i
	}
	offset := min(m.markedLines[modifyIndex].offset, elem.offset)
	length := max(elem.end(), m.markedLines[lastIndex].end()) - offset
	if lastIndex != modifyIndex {
		copy(m.markedLines[modifyIndex+1:], m.markedLines[lastIndex+1:])
		m.markedLines = m.markedLines[:len(m.markedLines)-lastIndex+modifyIndex]
	}
	m.markedLines[modifyIndex].offset = offset
	m.markedLines[modifyIndex].length = length
}

// 在 index 位置上插入元素 elem。
func (m *AxisMarker) insert(elem *line, index int) {
	if index == 0 {
		m.prepend(elem)
		return
	}
	if index == len(m.markedLines) {
		m.append(elem)
		return
	}
	m.expandCapacity(1)
	copy(m.markedLines[index+1:], m.markedLines[index:])
	m.markedLines[index] = elem
}

// 元素 elem 与 newElem 合并。
func (m *AxisMarker) merge(elem, newElem *line) {
	length := elem.offset - newElem.offset
	if length < 0 {
		length = newElem.length - length
	} else {
		length = length + elem.length
	}
	elem.offset = min(newElem.offset, elem.offset)
	elem.length = max(length, newElem.length)
}

// 缩小数组容量。
func (m *AxisMarker) shrinkCapacity() {
	lineLength := len(m.markedLines)
	if lineLength > 32 && cap(m.markedLines) > lineLength*2 {
		tmp := make([]*line, lineLength, int(float64(lineLength)*1.5))
		copy(tmp, m.markedLines)
		m.markedLines = tmp
	}
}

// 返回线段的右点。
func (l *line) end() int {
	return l.offset + l.length
}
