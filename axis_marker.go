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
	"strings"
	"sync"
)

// AxisMarker 坐标轴记录器。
type AxisMarker struct {
	lock         sync.RWMutex
	lineSegments []*lineSegment
}

type lineSegment struct {
	offset int64
	length int64
}

// Mark 记录某段为标记态。
func (m *AxisMarker) Mark(offset, length int64) {
	if length == 0 {
		return
	}
	if length < 0 {
		offset += length
		length = -length
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	index := m.searchOffset(offset)
	m.append(index, &lineSegment{offset: offset, length: length})
	m.clean(index)
}

// GetMaxMarkLine 获取从某点开始连续被标记的长度。
func (m *AxisMarker) GetMaxMarkLine(begin int64) int64 {
	if len(m.lineSegments) <= 0 {
		return 0
	}
	m.lock.RLock()
	defer m.lock.RUnlock()
	index := len(m.lineSegments) / 2
	topIndex := len(m.lineSegments) - 1
	lowIndex := 0
	tmp := 0
	for {
		cur := m.lineSegments[index]
		switch {
		case begin < cur.offset:
			if index == lowIndex {
				return 0
			}
			prev := m.lineSegments[index-1]
			if prev.rightPoint() < begin {
				return 0
			}
			if prev.offset <= begin {
				return prev.rightPoint() - begin
			}
			tmp = (lowIndex + index) / 2
			topIndex = index
			if tmp == index {
				index--
			} else {
				index = tmp
			}
		case begin > cur.offset:
			if index == topIndex {
				if cur.rightPoint() < begin {
					return 0
				}
				return cur.rightPoint() - begin
			}
			if m.lineSegments[index+1].offset > begin {
				if cur.rightPoint() < begin {
					return 0
				}
				return cur.rightPoint() - begin
			}
			tmp = (topIndex + index) / 2
			lowIndex = index
			if tmp == index {
				index++
			} else {
				index = tmp
			}
		case begin == cur.offset:
			return cur.length
		}
	}
}

func (m *AxisMarker) String() string {
	sb := make([]string, len(m.lineSegments))
	for i, v := range m.lineSegments {
		sb[i] = fmt.Sprintf("[%d, %d]", v.offset, v.length)
	}
	return "{" + strings.Join(sb, ", ") + "}"
}

func (m *AxisMarker) searchOffset(offset int64) int {
	if len(m.lineSegments) <= 0 {
		return 0
	}
	index := len(m.lineSegments) / 2
	topIndex := len(m.lineSegments) - 1
	lowIndex := 0
	tmp := 0
	for {
		cur := m.lineSegments[index]
		switch {
		case offset < cur.offset:
			if index == lowIndex || m.lineSegments[index-1].offset <= offset {
				return index
			}
			tmp = (lowIndex + index) / 2
			topIndex = index
			if tmp == index {
				index--
			} else {
				index = tmp
			}
		case offset > cur.offset:
			if index == topIndex || m.lineSegments[index+1].offset >= offset {
				return index + 1
			}
			tmp = (topIndex + index) / 2
			lowIndex = index
			if tmp == index {
				index++
			} else {
				index = tmp
			}
		case offset == cur.offset:
			return index
		}
	}
}

func (m *AxisMarker) append(index int, s *lineSegment) {
	if len(m.lineSegments) < cap(m.lineSegments) {
		m.lineSegments = append(m.lineSegments, &lineSegment{})
		var cur *lineSegment
		for i := len(m.lineSegments) - 1; i >= index && i != 0; i-- {
			cur = m.lineSegments[i]
			prev := m.lineSegments[i-1]
			cur.offset = prev.offset
			cur.length = prev.length
		}
		cur = m.lineSegments[index]
		cur.offset = s.offset
		cur.length = s.length
	} else {
		tmp := make([]*lineSegment, 0, len(m.lineSegments)+1)
		tmp = append(tmp, m.lineSegments[:index]...)
		tmp = append(tmp, s)
		m.lineSegments = append(tmp, m.lineSegments[index:]...)
	}
}

func (m *AxisMarker) clean(index int) {
	if len(m.lineSegments) <= 1 {
		return
	}
	cur := m.lineSegments[index]
	prevOverlap := false
	nextOverlap := false
	overlapNum := 1
	switch index {
	case 0:
		next := m.lineSegments[index+1]
		nextOverlap = next.offset <= cur.rightPoint()
	case len(m.lineSegments) - 1:
		prev := m.lineSegments[index-1]
		prevOverlap = prev.rightPoint() >= cur.offset
	default:
		prev := m.lineSegments[index-1]
		next := m.lineSegments[index+1]
		prevOverlap = prev.rightPoint() >= cur.offset
		nextOverlap = next.offset <= cur.rightPoint()
	}
	if nextOverlap {
		for i := index + 2; i < len(m.lineSegments); i++ {
			if m.lineSegments[i].offset <= cur.rightPoint() {
				overlapNum++
			} else {
				break
			}
		}
	}
	if prevOverlap && nextOverlap {
		pl := m.lineSegments[index-1]
		nl := m.lineSegments[index+overlapNum]
		pl.length = nl.rightPoint() - pl.offset
		m.lineSegments = append(m.lineSegments[:index], m.lineSegments[index+overlapNum+1:]...)
	} else if nextOverlap {
		nl := m.lineSegments[index+overlapNum]
		nl.length = nl.rightPoint() - cur.offset
		nl.offset = cur.offset
		m.lineSegments = append(m.lineSegments[:index], m.lineSegments[index+overlapNum:]...)
	} else if prevOverlap {
		pl := m.lineSegments[index-1]
		pl.length = cur.rightPoint() - pl.offset
		m.lineSegments = append(m.lineSegments[:index], m.lineSegments[index+1:]...)
	}
}

func (l *lineSegment) rightPoint() int64 {
	return l.offset + l.length
}
