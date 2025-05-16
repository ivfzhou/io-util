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
	"context"
	"errors"
	"io"
	"math/rand"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	iu "gitee.com/ivfzhou/io-util"
)

func TestNewMultiReadCloserToWriter(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			type writeInfo struct {
				offset int
				data   []byte
			}
			writeResult := make([]*writeInfo, 0, 4)
			lock := sync.Mutex{}
			send, wait := iu.NewMultiReadCloserToWriter(context.Background(), func(offset int, p []byte) {
				lock.Lock()
				defer lock.Unlock()
				bs := make([]byte, len(p))
				copy(bs, p)
				writeResult = append(writeResult, &writeInfo{offset: offset, data: bs})
			})
			atomic.StoreInt32(&CloseCount, 0)
			data, data1 := MakeByteArray(3)
			rcs := []*struct {
				offset int
				size   int
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					size:   len(data[0]),
					rc:     NewReader(data[0], nil, nil, nil),
				},
				{
					offset: len(data[0]),
					size:   len(data[1]),
					rc:     NewReader(data[1], nil, nil, nil),
				},
				{
					offset: len(data[1]) + len(data[0]),
					size:   len(data[2]),
					rc:     NewReader(data[2], nil, nil, nil),
				},
			}
			for _, v := range rcs {
				send(v.size, v.offset, v.rc)
			}
			err := wait()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			var result []byte
			slices.SortFunc(writeResult, func(a, b *writeInfo) int { return a.offset - b.offset })
			for _, v := range writeResult {
				result = append(result, v.data...)
			}
			if !bytes.Equal(result, data1) {
				t.Errorf("unexpected result: want %v, got %v", len(data1), len(result))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			ctx, cancel := NewCtxCancelWithError()
			expectedErr := errors.New("expected error")
			type writeInfo struct {
				offset int
				data   []byte
			}
			writeResult := make([]*writeInfo, 0, 4)
			lock := sync.Mutex{}
			send, wait := iu.NewMultiReadCloserToWriter(ctx, func(offset int, p []byte) {
				lock.Lock()
				defer lock.Unlock()
				bs := make([]byte, len(p))
				copy(bs, p)
				writeResult = append(writeResult, &writeInfo{offset: offset, data: bs})
			})
			atomic.StoreInt32(&CloseCount, 0)
			data, data1 := MakeByteArray(3)
			rcs := []*struct {
				offset int
				size   int
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					size:   len(data[0]),
					rc:     NewReader(data[0], nil, nil, nil),
				},
				{
					offset: len(data[0]),
					size:   len(data[1]),
					rc: NewReader(data[1], func() {
						time.Sleep(time.Millisecond * 10)
						cancel(expectedErr)
					}, nil, nil),
				},
				{
					offset: len(data[1]) + len(data[0]),
					size:   len(data[2]),
					rc:     NewReader(data[2], nil, nil, nil),
				},
			}
			for _, v := range rcs {
				send(v.size, v.offset, v.rc)
			}
			err := wait()
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			for _, v := range writeResult {
				if !bytes.Contains(data1, v.data) {
					t.Errorf("unexpected result: want %v, got %v", len(data1), len(v.data))
				}
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("读取失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			type writeInfo struct {
				offset int
				data   []byte
			}
			writeResult := make([]*writeInfo, 0, 4)
			lock := sync.Mutex{}
			send, wait := iu.NewMultiReadCloserToWriter(context.Background(), func(offset int, p []byte) {
				lock.Lock()
				defer lock.Unlock()
				writeResult = append(writeResult, &writeInfo{offset: offset, data: p})
			})
			expectedErr := errors.New("expected error")
			atomic.StoreInt32(&CloseCount, 0)
			data, data1 := MakeByteArray(3)
			rcs := []*struct {
				offset int
				size   int
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					size:   len(data[0]),
					rc:     NewReader(data[0], nil, nil, nil),
				},
				{
					offset: len(data[0]),
					size:   len(data[1]),
					rc:     NewReader(data[1], nil, nil, expectedErr),
				},
				{
					offset: len(data[1]) + len(data[0]),
					size:   len(data[2]),
					rc:     NewReader(data[2], nil, nil, nil),
				},
			}
			for _, v := range rcs {
				send(v.size, v.offset, v.rc)
			}
			err := wait()
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			slices.SortFunc(writeResult, func(a, b *writeInfo) int { return a.offset - b.offset })
			for _, v := range writeResult {
				if !bytes.Contains(data1, v.data) {
					t.Errorf("unexpected result: want %v, got %v", len(data1), len(v.data))
				}
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("大量数据", func(t *testing.T) {
		expectedResult := MakeBytes(1024*1024*100*(rand.Intn(5)+1) + 10)
		var resultMap sync.Map
		send, wait := iu.NewMultiReadCloserToWriter(context.Background(), func(off int, p []byte) {
			tmp := make([]byte, len(p))
			copy(tmp, p)
			resultMap.Store(off, tmp)
		})
		parts := Split(expectedResult)
		atomic.StoreInt32(&CloseCount, 0)
		for _, v := range parts {
			tmp := expectedResult[v.Offset:v.End]
			send(len(tmp), v.Offset, NewReader(tmp, nil, nil, nil))
		}
		err := wait()
		if err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		keys := make([]int, 0, len(parts))
		resultMap.Range(func(key, value any) bool {
			keys = append(keys, key.(int))
			return true
		})
		slices.SortFunc(keys, func(a, b int) int { return a - b })
		result := make([]byte, 0, len(expectedResult))
		for _, v := range keys {
			value, _ := resultMap.Load(v)
			result = append(result, value.([]byte)...)
		}
		if !bytes.Equal(expectedResult, result) {
			t.Errorf("unexpected result: want %v, got %v", len(expectedResult), len(result))
		}
		if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
			t.Errorf("unexpected close count: want 0, got %v", closeCount)
		}
	})
}
