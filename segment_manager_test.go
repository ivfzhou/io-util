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
	"runtime"
	"sync"
	"testing"

	iu "gitee.com/ivfzhou/io-util"
)

func TestSegmentManager(t *testing.T) {
	t.Run("一个段中写入和读取", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			m := &iu.SegmentManager{}

			expectedResult := []byte("hello world")
			n, err := m.WriteAt(expectedResult, 0)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), n)
			}

			result := make([]byte, len(expectedResult))
			n, err = io.ReadFull(m, result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(result) {
				t.Errorf("unexpected result: want %v, got %v", len(result), n)
			}
			if !bytes.Equal(result, expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", expectedResult, result)
			}

			m.Discard()
		}
	})

	t.Run("一个段中部写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			m := &iu.SegmentManager{}

			expectedResult := []byte("hello world")
			n, err := m.WriteAt(expectedResult, 10)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), n)
			}

			result := make([]byte, len(expectedResult))
			n, err = m.Read(result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 0 {
				t.Errorf("unexpected result: want 0, got %v", n)
			}

			m.Discard()
		}
	})

	t.Run("一个段中多次读取", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			m := &iu.SegmentManager{}

			expectedResult := []byte("hello world")
			n, err := m.WriteAt(expectedResult, 0)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), n)
			}

			readLength := len(expectedResult) / 2
			result := make([]byte, readLength)
			n, err = io.ReadFull(m, result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(result) {
				t.Errorf("unexpected result: want %v, got %v", len(result), n)
			}
			if !bytes.Equal(result, expectedResult[:n]) {
				t.Errorf("unexpected result: want %v, got %v", expectedResult[:n], result)
			}

			n, err = io.ReadFull(m, result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(result) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), n)
			}
			if !bytes.Equal(result, expectedResult[readLength:readLength+n]) {
				t.Errorf("unexpected result: want %v, got %v", expectedResult[readLength:readLength+n], result)
			}

			m.Discard()
		}
	})

	t.Run("一个段读取，刚好读完", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			m := &iu.SegmentManager{}

			expectedResult := make([]byte, iu.SegmentLength)
			for i := range expectedResult {
				expectedResult[i] = byte(rand.Intn(255))
			}
			n, err := m.WriteAt(expectedResult, 0)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), n)
			}

			result := make([]byte, len(expectedResult))
			n, err = io.ReadFull(m, result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(result) {
				t.Errorf("unexpected result: want %v, got %v", n, len(result))
			}
			if !bytes.Equal(result, expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), len(result))
			}

			m.Discard()
		}
	})

	t.Run("跨一个段读取，超过写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			writeLength := iu.SegmentLength + 10
			expectedResult := make([]byte, writeLength)
			for i := range expectedResult {
				expectedResult[i] = byte(rand.Int31n(1<<8 - 1))
			}

			m := &iu.SegmentManager{}
			n, err := m.WriteAt(expectedResult, 0)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), n)
			}

			readLength := iu.SegmentLength / 3 * 2
			result := make([]byte, readLength)
			n, err = io.ReadFull(m, result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(result) {
				t.Errorf("unexpected result: want %v, got %v", len(result), n)
			}
			if !bytes.Equal(result, expectedResult[:readLength]) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult[:readLength]), len(result))
			}

			remain := writeLength - readLength
			n, err = io.ReadFull(m, result[:remain])
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != remain {
				t.Errorf("unexpected result: want %v, got %v", remain, n)
			}
			if !bytes.Equal(result[:remain], expectedResult[readLength:]) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult[readLength:]), len(result[:remain]))
			}
		}
	})

	t.Run("在读取位置之前写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			m := &iu.SegmentManager{}

			expectedResult := []byte("hello world")
			n, err := m.WriteAt(expectedResult, 0)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), n)
			}

			result := make([]byte, len(expectedResult)/2)
			n, err = io.ReadFull(m, result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(result) {
				t.Errorf("unexpected result: want %v, got %v", len(result), n)
			}
			if !bytes.Equal(result, expectedResult[:len(expectedResult)/2]) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult[:len(expectedResult)/2]), len(result))
			}

			n, err = m.WriteAt(expectedResult, 1)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), n)
			}

			n, err = io.ReadFull(m, result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(result) {
				t.Errorf("unexpected result: want %v, got %v", len(result), n)
			}
			if !bytes.Equal(result, []byte("o wor")) {
				t.Errorf("unexpected result: want o wor, got %v", result)
			}

			n, err = io.ReadFull(m, result[:2])
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 2 {
				t.Errorf("unexpected result: want 2, got %v", n)
			}
			if !bytes.Equal(result[:n], []byte("ld")) {
				t.Errorf("unexpected result: want ld, got %v", result)
			}

			m.Discard()
		}
	})

	t.Run("跨多个段读取", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			m := &iu.SegmentManager{}

			expectedResult := make([]byte, iu.SegmentLength*3+40)
			for i := range expectedResult {
				expectedResult[i] = byte(rand.Int31n(1<<8 - 1))
			}

			n, err := m.WriteAt(expectedResult, 0)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", n, len(expectedResult))
			}

			result := make([]byte, iu.SegmentLength+10)
			n, err = io.ReadFull(m, result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(result) {
				t.Errorf("unexpected result: want %v, got %v", n, len(result))
			}
			if !bytes.Equal(result, expectedResult[:n]) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult[:n]), len(result))
			}

			n, err = io.ReadFull(m, result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(result) {
				t.Errorf("unexpected result: want %v, got %v", n, len(result))
			}
			if !bytes.Equal(result, expectedResult[n:n*2]) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult[n:n*2]), len(result))
			}

			n, err = io.ReadFull(m, result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(result) {
				t.Errorf("unexpected result: want %v, got %v", n, len(result))
			}
			if !bytes.Equal(result, expectedResult[n*2:n*3]) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult[n*2:n*3]), len(result))
			}

			n, err = io.ReadFull(m, result[:10])
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 10 {
				t.Errorf("unexpected result: want 10, got %v", n)
			}
			if !bytes.Equal(result[:n], expectedResult[len(expectedResult)-10:]) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult[len(expectedResult)-10:]), len(result[:n]))
			}

			m.Discard()
		}
	})

	t.Run("随机写入，每个位置只写一次", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			expectedResult := make([]byte, 1024*1024*2*(rand.Intn(5)+1)+10)
			for i := range expectedResult {
				expectedResult[i] = byte(rand.Int31n(1<<8 - 1))
			}

			parts := Split(expectedResult)

			m := &iu.SegmentManager{}
			go func() {
				for _, v := range parts {
					data := expectedResult[v.Offset:v.End]
					n, err := m.WriteAt(data, int64(v.Offset))
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if n != len(data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), n)
					}
				}
			}()

			result := make([]byte, len(expectedResult))
			n, err := io.ReadFull(m, result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(result) {
				t.Errorf("unexpected result: want %v, got %v", n, len(result))
			}
			if !bytes.Equal(result, expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), len(result))
			}

			m.Discard()

			expectedResult = nil
			result = nil
			parts = nil
			runtime.GC()
		}
	})

	t.Run("随机写入，同一个位置可能多次写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			expectedResult := make([]byte, 1024*1024*2*(rand.Intn(5)+1)+10)
			for i := range expectedResult {
				expectedResult[i] = byte(rand.Int31n(1<<8 - 1))
			}

			parts := Split(expectedResult)
			offset := rand.Intn(len(expectedResult))
			parts2 := Split(expectedResult[offset:])

			m := &iu.SegmentManager{}
			wg := &sync.WaitGroup{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, v := range parts2 {
					data := expectedResult[offset:][v.Offset:v.End]
					n, err := m.WriteAt(data, int64(offset+v.Offset))
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if n != len(data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), n)
					}
				}
				for _, v := range parts {
					data := expectedResult[v.Offset:v.End]
					n, err := m.WriteAt(data, int64(v.Offset))
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if n != len(data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), n)
					}
				}
			}()

			result := make([]byte, len(expectedResult))
			n, err := io.ReadFull(m, result)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(result) {
				t.Errorf("unexpected result: want %v, got %v", n, len(result))
			}
			if !bytes.Equal(result, expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), len(result))
			}

			m.Discard()

			wg.Wait()
			expectedResult = nil
			result = nil
			parts = nil
			parts2 = nil
			runtime.GC()
		}
	})
}
