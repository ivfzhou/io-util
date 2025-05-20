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
	"fmt"
	"io"
	"maps"
	"math/rand"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	iu "gitee.com/ivfzhou/io-util"
)

func TestMultiReadCloserToWriterAt(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			resultMap := make(map[int64][]byte)
			lock := sync.Mutex{}
			wa := &writeAtFunc{func(p []byte, off int64) (int, error) {
				if len(p) <= 0 {
					return 0, nil
				}
				lock.Lock()
				defer lock.Unlock()
				index := rand.Intn(len(p)) + 1
				bs := make([]byte, len(p[:index]))
				copy(bs, p[:index])
				resultMap[off] = bs
				return index, nil
			}}
			send, wait := iu.NewMultiReadCloserToWriterAt(context.Background(), wa)
			data, data1 := MakeByteArray(3)
			atomic.StoreInt32(&CloseCount, 0)
			rcs := []*struct {
				offset int64
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					rc:     NewReader(data[0], nil, nil, nil),
				},
				{
					offset: int64(len(data[0])),
					rc:     NewReader(data[1], nil, nil, nil),
				},
				{
					offset: int64(len(data[1]) + len(data[0])),
					rc:     NewReader(data[2], nil, nil, nil),
				},
			}
			wg := sync.WaitGroup{}
			wg.Add(len(rcs))
			for _, v := range rcs {
				go func(rc io.ReadCloser, offset int64) {
					defer wg.Done()
					err := send(rc, offset)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
				}(v.rc, v.offset)
			}
			wg.Wait()
			fastExit := rand.Intn(2) == 1
			err := wait(fastExit)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			keys := slices.SortedFunc(maps.Keys(resultMap), func(x, y int64) int {
				return int(x - y)
			})
			var result []byte
			for _, v := range keys {
				result = append(result, resultMap[v]...)
			}
			if !bytes.Equal(data1, result) {
				t.Errorf("unexpected result: want %v, got %v", len(data1), len(result))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("读取发生错误", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			resultMap := make(map[int64][]byte)
			lock := sync.Mutex{}
			wa := &writeAtFunc{func(p []byte, off int64) (int, error) {
				if len(p) <= 0 {
					return 0, nil
				}
				lock.Lock()
				defer lock.Unlock()
				index := rand.Intn(len(p)) + 1
				resultMap[off] = p[:index]
				return index, nil
			}}
			send, wait := iu.NewMultiReadCloserToWriterAt(context.Background(), wa)
			atomic.StoreInt32(&CloseCount, 0)
			expectedErr := fmt.Errorf("expected error")
			data, data1 := MakeByteArray(3)
			rcs := []*struct {
				offset int64
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					rc:     NewReader(data[0], nil, nil, nil),
				},
				{
					offset: int64(len(data[0])),
					rc:     NewReader(data[1], nil, nil, expectedErr),
				},
				{
					offset: int64(len(data[0]) + len(data[1])),
					rc:     NewReader(data[2], nil, nil, nil),
				},
			}
			wg := sync.WaitGroup{}
			wg.Add(len(rcs))
			for _, v := range rcs {
				go func(rc io.ReadCloser, offset int64) {
					defer wg.Done()
					err := send(rc, offset)
					if err != nil && !errors.Is(err, expectedErr) {
						t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
					}
				}(v.rc, v.offset)
			}
			wg.Wait()
			fastExit := rand.Intn(2) == 1
			err := wait(fastExit)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if fastExit {
				time.Sleep(time.Millisecond * 100)
			}
			keys := slices.SortedFunc(maps.Keys(resultMap), func(x, y int64) int { return int(x - y) })
			var result []byte
			prevEnd := int64(-1)
			for _, v := range keys {
				if prevEnd >= 0 && prevEnd != v {
					continue
				}
				result = append(result, resultMap[v]...)
				prevEnd = v + int64(len(resultMap[v]))
			}
			if !bytes.Contains(data1, result) {
				t.Errorf("unexpected result: want %v, got %v", len(data1), result)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("写入发生错误", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			resultMap := make(map[int64][]byte)
			lock := sync.Mutex{}
			occurErrorIndex := rand.Intn(3)
			writeTimes := 0
			expectedErr := fmt.Errorf("expected error")
			wa := &writeAtFunc{func(p []byte, off int64) (int, error) {
				if len(p) <= 0 {
					return 0, nil
				}
				lock.Lock()
				defer lock.Unlock()
				index := rand.Intn(len(p)) + 1
				bs := make([]byte, index)
				copy(bs, p[:index])
				resultMap[off] = bs
				if occurErrorIndex == writeTimes {
					return index, expectedErr
				}
				writeTimes++
				return index, nil
			}}
			send, wait := iu.NewMultiReadCloserToWriterAt(context.Background(), wa)
			data, data1 := MakeByteArray(3)
			atomic.StoreInt32(&CloseCount, 0)
			rcs := []*struct {
				offset int64
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					rc:     NewReader(data[0], nil, nil, nil),
				},
				{
					offset: int64(len(data[0])),
					rc:     NewReader(data[1], nil, nil, nil),
				},
				{
					offset: int64(len(data[1]) + len(data[0])),
					rc:     NewReader(data[2], nil, nil, nil),
				},
			}
			wg := sync.WaitGroup{}
			wg.Add(len(rcs))
			for _, v := range rcs {
				go func(rc io.ReadCloser, offset int64) {
					defer wg.Done()
					err := send(rc, offset)
					if err != nil && !errors.Is(err, expectedErr) {
						t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
					}
				}(v.rc, v.offset)
			}
			wg.Wait()
			fastExit := rand.Intn(2) == 1
			err := wait(fastExit)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if fastExit {
				time.Sleep(time.Millisecond * 100)
			}
			keys := slices.SortedFunc(maps.Keys(resultMap), func(x, y int64) int { return int(x - y) })
			var result []byte
			prevEnd := int64(-1)
			for _, v := range keys {
				if prevEnd >= 0 && prevEnd != v {
					continue
				}
				result = append(result, resultMap[v]...)
				prevEnd = v + int64(len(resultMap[v]))
			}
			if !bytes.Contains(data1, result) {
				t.Errorf("unexpected result: want %v, got %v", len(data1), len(result))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			ctx, cancel := NewCtxCancelWithError()
			result := make(map[int64][]byte)
			lock := sync.Mutex{}
			wa := &writeAtFunc{func(p []byte, off int64) (int, error) {
				if len(p) <= 0 {
					return 0, nil
				}
				lock.Lock()
				defer lock.Unlock()
				result[off] = p
				return len(p), nil
			}}
			send, wait := iu.NewMultiReadCloserToWriterAt(ctx, wa)
			atomic.StoreInt32(&CloseCount, 0)
			cancelIndex := rand.Intn(3)
			expectedErr := fmt.Errorf("expected error")
			data, data1 := MakeByteArray(3)
			rcs := []*struct {
				offset int64
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					rc: NewReader(data[0], func() {
						time.Sleep(time.Millisecond * 10)
						if cancelIndex == 0 {
							cancel(expectedErr)
						}
					}, nil, nil),
				},
				{
					offset: 13,
					rc: NewReader(data[1], func() {
						if cancelIndex == 1 {
							cancel(expectedErr)
						}
						time.Sleep(time.Millisecond * 10)
					}, nil, nil),
				},
				{
					offset: 29,
					rc: NewReader(data[2], func() {
						if cancelIndex == 2 {
							cancel(expectedErr)
						}
						time.Sleep(time.Millisecond * 10)
					}, nil, nil),
				},
			}
			wg := sync.WaitGroup{}
			wg.Add(len(rcs))
			for _, v := range rcs {
				go func(rc io.ReadCloser, offset int64) {
					defer wg.Done()
					err := send(rc, offset)
					if err != nil && !errors.Is(err, expectedErr) {
						t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
					}
				}(v.rc, v.offset)
			}
			wg.Wait()
			fastExit := rand.Intn(2) == 1
			err := wait(fastExit)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if fastExit {
				time.Sleep(time.Millisecond * 100)
			}
			offsets := slices.SortedFunc(maps.Keys(result), func(x, y int64) int { return int(x - y) })
			var bs []byte
			prevEnd := int64(-1)
			for _, v := range offsets {
				if prevEnd >= 0 && prevEnd != v {
					continue
				}
				bs = append(bs, result[v]...)
				prevEnd = v + int64(len(result[v]))
			}
			if !bytes.Contains(data1, bs) {
				t.Errorf("unexpected result: want %v, got %v", len(data1), len(bs))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("关闭读取失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			expectedErr := fmt.Errorf("expected error")
			result := make(map[int64][]byte)
			lock := sync.Mutex{}
			wa := &writeAtFunc{func(p []byte, off int64) (int, error) {
				if len(p) <= 0 {
					return 0, nil
				}
				lock.Lock()
				defer lock.Unlock()
				result[off] = p
				return len(p), nil
			}}
			send, wait := iu.NewMultiReadCloserToWriterAt(context.Background(), wa)
			data, data1 := MakeByteArray(3)
			atomic.StoreInt32(&CloseCount, 0)
			rcs := []*struct {
				offset int64
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					rc: NewReader(data[0], func() {
						time.Sleep(time.Millisecond * 10)
					}, nil, nil),
				},
				{
					offset: 13,
					rc: NewReader(data[1], func() {
						time.Sleep(time.Millisecond * 10)
					}, nil, nil),
				},
				{
					offset: 29,
					rc: NewReader(data[2], func() {
						time.Sleep(time.Millisecond * 10)
					}, expectedErr, nil),
				},
			}
			wg := sync.WaitGroup{}
			wg.Add(len(rcs))
			for _, v := range rcs {
				go func(rc io.ReadCloser, offset int64) {
					defer wg.Done()
					err := send(rc, offset)
					if err != nil && !errors.Is(err, expectedErr) {
						t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
					}
				}(v.rc, v.offset)
			}
			wg.Wait()
			fastExit := rand.Intn(2) == 1
			err := wait(fastExit)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if fastExit {
				time.Sleep(time.Millisecond * 100)
			}
			offsets := slices.SortedFunc(maps.Keys(result), func(x, y int64) int { return int(x - y) })
			var bs []byte
			prevEnd := int64(-1)
			for _, v := range offsets {
				if prevEnd >= 0 && prevEnd != v {
					continue
				}
				bs = append(bs, result[v]...)
				prevEnd = v + int64(len(result[v]))
			}
			if !bytes.Contains(data1, bs) {
				t.Errorf("unexpected result: want %v, got %v", len(data1), bs)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("大量数据", func(t *testing.T) {
		data := MakeBytes(1024*1024*100*(rand.Intn(5)+1) + 13)
		var resultMap sync.Map
		wa := &writeAtFunc{func(p []byte, off int64) (int, error) {
			if len(p) <= 0 {
				return 0, nil
			}
			index := rand.Intn(len(p)) + 1
			tmp := make([]byte, index)
			copy(tmp, p)
			resultMap.Store(off, tmp)
			return index, nil
		}}
		send, wait := iu.NewMultiReadCloserToWriterAt(context.Background(), wa)
		parts := Split(data)
		atomic.StoreInt32(&CloseCount, 0)
		wg := sync.WaitGroup{}
		wg.Add(len(parts))
		for _, v := range parts {
			go func(v *Part) {
				defer wg.Done()
				err := send(NewReader(data[v.Offset:v.End], nil, nil, nil), int64(v.Offset))
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
			}(v)
		}
		wg.Wait()
		fastExit := rand.Intn(2) == 1
		err := wait(fastExit)
		if err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if fastExit {
			time.Sleep(time.Millisecond * 100)
		}
		keys := make([]int64, 0, len(parts))
		resultMap.Range(func(key, value any) bool {
			keys = append(keys, key.(int64))
			return true
		})
		slices.SortFunc(keys, func(a, b int64) int { return int(a - b) })
		result := make([]byte, 0, len(data))
		for _, v := range keys {
			value, _ := resultMap.Load(v)
			result = append(result, value.([]byte)...)
		}
		if !bytes.Equal(data, result) {
			t.Errorf("unexpected result: want %v, got %v", len(data), len(result))
		}
		if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
			t.Errorf("unexpected close close count: want 0, got %v", closeCount)
		}
	})
}
