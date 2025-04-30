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

package io_util_test

import (
	"bytes"
	"io"
	"math/rand"
	"testing"

	iu "gitee.com/ivfzhou/io-util"
)

func TestSegmentManager(t *testing.T) {
	t.Run("一个段中写入和读取", func(t *testing.T) {
		m := &iu.SegmentManager{}

		data := []byte("hello world")
		n, err := m.WriteAt(data, 0)
		if err != nil {
			t.Errorf("WriteAt() error %v", err)
		}
		if n != len(data) {
			t.Errorf("WriteAt() returns %d, want %d", n, len(data))
		}

		expected := make([]byte, len(data))
		n, err = io.ReadFull(m, expected)
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != len(expected) {
			t.Errorf("Read() returns %d, want %d", n, len(expected))
		}
		if !bytes.Equal(expected, data) {
			t.Errorf("Read() returns %v, want %v", expected, data)
		}

		m.Discard()
	})

	t.Run("一个段中部写入", func(t *testing.T) {
		m := &iu.SegmentManager{}

		data := []byte("hello world")
		n, err := m.WriteAt(data, 10)
		if err != nil {
			t.Errorf("WriteAt() error %v", err)
		}
		if n != len(data) {
			t.Errorf("WriteAt() returns %d, want %d", n, len(data))
		}

		expected := make([]byte, len(data))
		n, err = m.Read(expected)
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != 0 {
			t.Errorf("Read() returns %d, want %d", n, 0)
		}

		m.Discard()
	})

	t.Run("一个段中多次读取", func(t *testing.T) {
		m := &iu.SegmentManager{}

		data := []byte("hello world")
		n, err := m.WriteAt(data, 0)
		if err != nil {
			t.Errorf("WriteAt() error %v", err)
		}
		if n != len(data) {
			t.Errorf("WriteAt() returns %d, want %d", n, len(data))
		}

		readLength := len(data) / 2
		expected := make([]byte, readLength)
		n, err = io.ReadFull(m, expected)
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != len(expected) {
			t.Errorf("Read() returns %d, want %d", n, len(expected))
		}
		if !bytes.Equal(expected, data[:n]) {
			t.Errorf("Read() returns %s, want %s", expected, data[:n])
		}

		n, err = io.ReadFull(m, expected)
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != len(expected) {
			t.Errorf("Read() returns %d, want %d", n, len(data))
		}
		if !bytes.Equal(expected, data[readLength:readLength+n]) {
			t.Errorf("Read() returns %s, want %s", expected, data[readLength:readLength+n])
		}

		m.Discard()
	})

	t.Run("一个段读取，刚好读完", func(t *testing.T) {
		m := &iu.SegmentManager{}

		data := make([]byte, iu.SegmentLength)
		for i := range data {
			data[i] = byte(rand.Intn(255))
		}
		n, err := m.WriteAt(data, 0)
		if err != nil {
			t.Errorf("WriteAt() error %v", err)
		}
		if n != len(data) {
			t.Errorf("WriteAt() returns %d, want %d", n, len(data))
		}

		expected := make([]byte, len(data))
		n, err = io.ReadFull(m, expected)
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != len(expected) {
			t.Errorf("Read() returns %d, want %d", n, len(expected))
		}
		if !bytes.Equal(expected, data) {
			t.Errorf("Read() returns %d, want %d", len(expected), len(data))
		}

		m.Discard()
	})

	t.Run("跨一个段读取，超过写入", func(t *testing.T) {
		writeLength := iu.SegmentLength + 10
		data := make([]byte, writeLength)
		for i := range data {
			data[i] = byte(rand.Int31n(1<<8 - 1))
		}

		m := &iu.SegmentManager{}
		n, err := m.WriteAt(data, 0)
		if err != nil {
			t.Errorf("WriteAt() error %v", err)
		}
		if n != len(data) {
			t.Errorf("WriteAt() returns %d, want %d", n, len(data))
		}

		readLength := iu.SegmentLength / 3 * 2
		expected := make([]byte, readLength)
		n, err = io.ReadFull(m, expected)
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != len(expected) {
			t.Errorf("Read() returns %d, want %d", n, len(expected))
		}
		if !bytes.Equal(expected, data[:readLength]) {
			t.Errorf("Read() returns %d, want %d", len(expected), len(data[:readLength]))
		}

		remain := writeLength - readLength
		n, err = io.ReadFull(m, expected[:remain])
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != remain {
			t.Errorf("Read() returns %d, want %d", n, remain)
		}
		if !bytes.Equal(expected[:remain], data[readLength:]) {
			t.Errorf("Read() returns %d, want %d", len(expected[:remain]), len(data[readLength:]))
		}
	})

	t.Run("在读取位置之前写入", func(t *testing.T) {
		m := &iu.SegmentManager{}

		data := []byte("hello world")
		n, err := m.WriteAt(data, 0)
		if err != nil {
			t.Errorf("WriteAt() error %v", err)
		}
		if n != len(data) {
			t.Errorf("WriteAt() returns %d, want %d", n, len(data))
		}

		expected := make([]byte, len(data)/2)
		n, err = io.ReadFull(m, expected)
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != len(expected) {
			t.Errorf("Read() returns %d, want %d", n, len(expected))
		}
		if !bytes.Equal(expected, data[:len(data)/2]) {
			t.Errorf("Read() returns %d, want %d", len(expected), len(data[:len(data)/2]))
		}

		n, err = m.WriteAt(data, 1)
		if err != nil {
			t.Errorf("WriteAt() error %v", err)
		}
		if n != len(data) {
			t.Errorf("WriteAt() returns %d, want %d", n, len(data))
		}

		n, err = io.ReadFull(m, expected)
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != len(expected) {
			t.Errorf("Read() returns %d, want %d", n, len(expected))
		}
		if !bytes.Equal(expected, []byte("o wor")) {
			t.Errorf("Read() returns %s, want %s", expected, "o wor")
		}

		n, err = io.ReadFull(m, expected[:2])
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != 2 {
			t.Errorf("Read() returns %d, want %d", n, 2)
		}
		if !bytes.Equal(expected[:n], []byte("ld")) {
			t.Errorf("Read() returns %s, want %s", expected, "ld")
		}

		m.Discard()
	})

	t.Run("跨多个段读取", func(t *testing.T) {
		m := &iu.SegmentManager{}

		data := make([]byte, iu.SegmentLength*3+40)
		for i := range data {
			data[i] = byte(rand.Int31n(1<<8 - 1))
		}

		n, err := m.WriteAt(data, 0)
		if err != nil {
			t.Errorf("WriteAt() error %v", err)
		}
		if n != len(data) {
			t.Errorf("WriteAt() returns %d, want %d", n, len(data))
		}

		expected := make([]byte, iu.SegmentLength+10)
		n, err = io.ReadFull(m, expected)
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != len(expected) {
			t.Errorf("Read() returns %d, want %d", n, len(expected))
		}
		if !bytes.Equal(expected, data[:n]) {
			t.Errorf("Read() returns %d, want %d", len(expected), len(data[:n]))
		}

		n, err = io.ReadFull(m, expected)
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != len(expected) {
			t.Errorf("Read() returns %d, want %d", n, len(expected))
		}
		if !bytes.Equal(expected, data[n:n*2]) {
			t.Errorf("Read() returns %d, want %d", len(expected), len(data[n:n*2]))
		}

		n, err = io.ReadFull(m, expected)
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != len(expected) {
			t.Errorf("Read() returns %d, want %d", n, len(expected))
		}
		if !bytes.Equal(expected, data[n*2:n*3]) {
			t.Errorf("Read() returns %d, want %d", len(expected), len(data[n*2:n*3]))
		}

		n, err = io.ReadFull(m, expected[:10])
		if err != nil {
			t.Errorf("Read() error %v", err)
		}
		if n != 10 {
			t.Errorf("Read() returns %d, want %d", n, 10)
		}
		if !bytes.Equal(expected[:n], data[len(data)-10:]) {
			t.Errorf("Read() returns %d, want %d", len(expected[:n]), len(data[len(data)-10:]))
		}

		m.Discard()
	})

	t.Run("随机写入，每个位置只写一次", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			data := make([]byte, 1024*1024*(rand.Intn(150)+1)+10)
			for i := range data {
				data[i] = byte(rand.Int31n(1<<8 - 1))
			}

			type part struct {
				offset int
				data   []byte
			}
			parts := make([]*part, 0, 10)
			var fn func(arr []byte, offset int)
			fn = func(arr []byte, offset int) {
				if len(arr) <= 0 {
					return
				}
				start := len(arr) / (rand.Intn(3) + 1)
				length := len(arr[start:]) / (rand.Intn(3) + 1)
				if length > 0 {
					parts = append(parts, &part{offset + start, arr[start : start+length]})
				}
				fn(arr[:start], offset)
				fn(arr[start+length:], start+length+offset)
			}
			fn(data, 0)

			m := &iu.SegmentManager{}
			go func() {
				for _, v := range parts {
					n, err := m.WriteAt(v.data, int64(v.offset))
					if err != nil {
						t.Errorf("WriteAt() error %v", err)
					}
					if n != len(v.data) {
						t.Errorf("WriteAt() returns %d, want %d", n, len(v.data))
					}
				}
			}()

			expected := make([]byte, len(data))
			n, err := io.ReadFull(m, expected)
			if err != nil {
				t.Errorf("Read() error %v", err)
			}
			if n != len(expected) {
				t.Errorf("Read() returns %d, want %d", n, len(expected))
			}
			if !bytes.Equal(expected, data) {
				t.Errorf("Read() returns %d, want %d", len(data), len(expected))
			}

			m.Discard()
		}
	})

	t.Run("随机写入，同一个位置可能多次写入", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			data := make([]byte, 1024*1024*(rand.Intn(150)+1)+10)
			for i := range data {
				data[i] = byte(rand.Int31n(1<<8 - 1))
			}

			type part struct {
				offset int
				data   []byte
			}
			parts := make([]*part, 0, 10)
			var fn func(arr []byte, offset int)
			fn = func(arr []byte, offset int) {
				if len(arr) <= 0 {
					return
				}
				start := len(arr) / (rand.Intn(3) + 1)
				length := len(arr[start:]) / (rand.Intn(3) + 1)
				if length > 0 {
					parts = append(parts, &part{offset + start, arr[start : start+length]})
				}
				fn(arr[:start], offset)
				fn(arr[start+length:], start+length+offset)
			}
			fn(data, 0)
			length := len(data) / (rand.Intn(3) + 1)
			offset := rand.Intn(max(len(data)-length, 1))
			data2 := data[offset : offset+length]
			fn(data2, offset)

			m := &iu.SegmentManager{}
			go func() {
				for _, v := range parts {
					n, err := m.WriteAt(v.data, int64(v.offset))
					if err != nil {
						t.Errorf("WriteAt() error %v", err)
					}
					if n != len(v.data) {
						t.Errorf("WriteAt() returns %d, want %d", n, len(v.data))
					}
				}
			}()

			expected := make([]byte, len(data))
			n, err := io.ReadFull(m, expected)
			if err != nil {
				t.Errorf("Read() error %v", err)
			}
			if n != len(expected) {
				t.Errorf("Read() returns %d, want %d", n, len(expected))
			}
			if !bytes.Equal(expected, data) {
				t.Errorf("Read() returns %d, want %d", len(data), len(expected))
			}

			m.Discard()
		}
	})
}
