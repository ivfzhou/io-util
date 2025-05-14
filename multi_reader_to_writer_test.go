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
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	iu "gitee.com/ivfzhou/io-util"
)

func TestNewMultiReadCloserToWriter(t *testing.T) {
	t.Run("正常读取", func(t *testing.T) {
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

			atomic.StoreInt32(&CloseCount, 0)
			rcs := []*struct {
				offset int
				size   int
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					size:   13,
					rc:     newClosePerception(strings.NewReader("hello world. ")),
				},
				{
					offset: 13,
					size:   16,
					rc:     newClosePerception(strings.NewReader("this is a test. ")),
				},
				{
					offset: 29,
					size:   6,
					rc:     newClosePerception(strings.NewReader("yes ok")),
				},
			}
			for _, v := range rcs {
				send(v.size, v.offset, v.rc)
			}
			err := wait()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}

			result := ""
			expectedResult := "hello world. this is a test. yes ok"
			slices.SortFunc(writeResult, func(a, b *writeInfo) int { return a.offset - b.offset })
			for _, v := range writeResult {
				result += string(v.data)
			}
			if result != expectedResult {
				t.Errorf("unexpected result: want %v, got %v", expectedResult, result)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			ctx, cancel := newCtxCancelWithError()
			expectedResult := "hello world. this is a test. yes ok"
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
				writeResult = append(writeResult, &writeInfo{offset: offset, data: p})
			})

			atomic.StoreInt32(&CloseCount, 0)
			rcs := []*struct {
				offset int
				size   int
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					size:   13,
					rc:     newClosePerception(strings.NewReader("hello world. ")),
				},
				{
					offset: 13,
					size:   16,
					rc: newReadInterceptor(func() {
						time.Sleep(time.Millisecond * 10)
						cancel(expectedErr)
					}, newClosePerception(strings.NewReader("this is a test. "))),
				},
				{
					offset: 29,
					size:   6,
					rc:     newClosePerception(strings.NewReader("yes ok")),
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
				if !strings.Contains(expectedResult, string(v.data)) {
					t.Errorf("unexpected result: want %v, got %v", expectedResult, string(v.data))
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
			rcs := []*struct {
				offset int
				size   int
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					size:   13,
					rc:     newClosePerception(strings.NewReader("hello world. ")),
				},
				{
					offset: 13,
					size:   16,
					rc:     newErrorReader(expectedErr, []byte("")),
				},
				{
					offset: 29,
					size:   6,
					rc:     newClosePerception(strings.NewReader("yes ok")),
				},
			}
			for _, v := range rcs {
				send(v.size, v.offset, v.rc)
			}
			err := wait()
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}

			expectedResult := "hello world. this is a test. yes ok"
			slices.SortFunc(writeResult, func(a, b *writeInfo) int { return a.offset - b.offset })
			for _, v := range writeResult {
				if !strings.Contains(expectedResult, string(v.data)) {
					t.Errorf("unexpected result: want %v, got %v", expectedResult, string(v.data))
				}
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("大量数据", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			expectedResult := make([]byte, 1024*1024*10*(rand.Intn(5)+1)+10)
			for i := range expectedResult {
				expectedResult[i] = byte(rand.Intn(256))
			}

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
				send(len(tmp), v.Offset, newClosePerception(bytes.NewReader(tmp)))
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

			expectedResult = nil
			parts = nil
			keys = nil
			result = nil
			resultMap.Clear()
			runtime.GC()
		}
	})
}

func TestMultiReadCloserToWriterAt(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			resultMap := make(map[int64][]byte)
			lock := sync.Mutex{}
			wa := &writerAt{func(p []byte, off int64) (int, error) {
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
			rcs := []*struct {
				offset int64
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					rc:     newClosePerception(strings.NewReader("hello world. ")),
				},
				{
					offset: 13,
					rc:     newClosePerception(strings.NewReader("this is a test. ")),
				},
				{
					offset: 29,
					rc:     newClosePerception(strings.NewReader("yes ok")),
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
			result := ""
			for _, v := range keys {
				result += string(resultMap[v])
			}
			expectedResult := "hello world. this is a test. yes ok"
			if result != expectedResult {
				t.Errorf("unexpected result: want %v, got %v", expectedResult, result)
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
			wa := &writerAt{func(p []byte, off int64) (int, error) {
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
			rcs := []*struct {
				offset int64
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					rc:     newClosePerception(strings.NewReader("hello world. ")),
				},
				{
					offset: 13,
					rc:     newErrorReader(expectedErr, []byte("this is a test. ")),
				},
				{
					offset: 29,
					rc:     newClosePerception(strings.NewReader("yes ok")),
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
			result := ""
			prevEnd := int64(-1)
			for _, v := range keys {
				if prevEnd >= 0 && prevEnd != v {
					continue
				}
				result += string(resultMap[v])
				prevEnd = v + int64(len(resultMap[v]))
			}
			expectedResult := "hello world. this is a test. yes ok"
			if !strings.Contains(expectedResult, result) {
				t.Errorf("unexpected result: want %v, got %v", expectedResult, result)
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
			wa := &writerAt{func(p []byte, off int64) (int, error) {
				if len(p) <= 0 {
					return 0, nil
				}
				lock.Lock()
				defer lock.Unlock()
				index := rand.Intn(len(p)) + 1
				resultMap[off] = p[:index]
				if occurErrorIndex == writeTimes {
					return index, expectedErr
				}
				writeTimes++
				return index, nil
			}}
			send, wait := iu.NewMultiReadCloserToWriterAt(context.Background(), wa)

			expectedResult := "hello world. this is a test. yes ok"
			atomic.StoreInt32(&CloseCount, 0)
			rcs := []*struct {
				offset int64
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					rc:     newClosePerception(strings.NewReader("hello world. ")),
				},
				{
					offset: 13,
					rc:     newClosePerception(strings.NewReader("this is a test. ")),
				},
				{
					offset: 29,
					rc:     newClosePerception(strings.NewReader("yes ok")),
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
			keys := slices.SortedFunc(maps.Keys(resultMap), func(x int64, y int64) int { return int(x - y) })
			result := ""
			prevEnd := int64(-1)
			for _, v := range keys {
				if prevEnd >= 0 && prevEnd != v {
					continue
				}
				result += string(resultMap[v])
				prevEnd = v + int64(len(resultMap[v]))
			}
			if !strings.Contains(expectedResult, result) {
				t.Errorf("unexpected result: want %v, got %v", expectedResult, result)
			}

			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			ctx, cancel := newCtxCancelWithError()
			result := make(map[int64][]byte)
			lock := sync.Mutex{}
			wa := &writerAt{func(p []byte, off int64) (int, error) {
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
			rcs := []*struct {
				offset int64
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					rc: newReadInterceptor(func() {
						time.Sleep(time.Millisecond * 10)
						if cancelIndex == 0 {
							cancel(expectedErr)
						}
					}, newClosePerception(strings.NewReader("hello world. "))),
				},
				{
					offset: 13,
					rc: newReadInterceptor(func() {
						if cancelIndex == 1 {
							cancel(expectedErr)
						}
						time.Sleep(time.Millisecond * 10)
					}, newClosePerception(strings.NewReader("this is a test. "))),
				},
				{
					offset: 29,
					rc: newReadInterceptor(func() {
						if cancelIndex == 2 {
							cancel(expectedErr)
						}
						time.Sleep(time.Millisecond * 10)
					}, newClosePerception(strings.NewReader("yes ok"))),
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

			expectedResult := "hello world. this is a test. yes ok"
			if !strings.Contains(expectedResult, string(bs)) {
				t.Errorf("unexpected result: want %v, got %v", expectedResult, string(bs))
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
			wa := &writerAt{func(p []byte, off int64) (int, error) {
				if len(p) <= 0 {
					return 0, nil
				}
				lock.Lock()
				defer lock.Unlock()
				result[off] = p
				return len(p), nil
			}}
			send, wait := iu.NewMultiReadCloserToWriterAt(context.Background(), wa)

			atomic.StoreInt32(&CloseCount, 0)
			rcs := []*struct {
				offset int64
				rc     io.ReadCloser
			}{
				{
					offset: 0,
					rc: newReadInterceptor(func() {
						time.Sleep(time.Millisecond * 10)
					}, newClosePerception(strings.NewReader("hello world. "))),
				},
				{
					offset: 13,
					rc: newReadInterceptor(func() {
						time.Sleep(time.Millisecond * 10)
					}, newClosePerception(strings.NewReader("this is a test. "))),
				},
				{
					offset: 29,
					rc: newReadInterceptor(func() {
						time.Sleep(time.Millisecond * 10)
					}, newErrorCloseReader(expectedErr, nil)),
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
			expectedResult := "hello world. this is a test. yes ok"
			if !strings.Contains(expectedResult, string(bs)) {
				t.Errorf("unexpected result: want %v, got %v", expectedResult, string(bs))
			}

			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("大量数据", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			data := make([]byte, 1024*1024*(rand.Intn(5)+1)+10)
			for i := range data {
				data[i] = byte(rand.Intn(256))
			}

			var resultMap sync.Map
			wa := &writerAt{func(p []byte, off int64) (int, error) {
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
					err := send(newClosePerception(bytes.NewReader(data[v.Offset:v.End])), int64(v.Offset))
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

			data = nil
			parts = nil
			resultMap.Clear()
			keys = nil
			result = nil
			runtime.GC()
		}
	})
}
