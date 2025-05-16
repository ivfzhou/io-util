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
	"sort"
	"sync"
	"sync/atomic"
	"testing"

	iu "gitee.com/ivfzhou/io-util"
)

func ExampleNewMultiReadCloserToReader() {
	ctx := context.Background()
	var rc1 io.ReadCloser
	var rc2 io.ReadCloser
	r, add, endAdd := iu.NewMultiReadCloserToReader(ctx, rc1, rc2)

	// 继续添加 Reader。
	go func() {
		var rc3 io.ReadCloser
		var rc4 io.ReadCloser
		err := add(rc3)
		err = add(rc4)
		_ = err
		// 处理 err。

		// 添加完所有 Reader 后调用。
		endAdd()
	}()

	// 将所有的 Reader 数据按顺序读出。
	bs, err := io.ReadAll(r)
	_, _ = bs, err
}

func TestNewMultiReadCloserToReader(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			data, data2 := MakeByteArray(3)
			rcs := make([]io.ReadCloser, 0, 4)
			rcs = append(rcs, NewReader(data[0], nil, nil, nil))
			rcs = append(rcs, NewReader(data[1], nil, nil, nil))
			rcs = append(rcs, NewReader(data[2], nil, nil, nil))
			r, add, endAdd := iu.NewMultiReadCloserToReader(context.Background(), rcs...)
			data3, data4 := MakeByteArray(3)
			go func() {
				rcs = append(rcs[:0], NewReader(data3[0], nil, nil, nil))
				rcs = append(rcs, NewReader(data3[1], nil, nil, nil))
				rcs = append(rcs, NewReader(data3[2], nil, nil, nil))
				for i := range rcs {
					err := add(rcs[i])
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
				}
				endAdd()
			}()
			data5 := append(data2, data4...)
			result, err := io.ReadAll(r)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(data5, result) {
				t.Errorf("unexpected result: want %v, got %v", len(data)+len(data2), len(result))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("没有数据", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			r, _, endAdd := iu.NewMultiReadCloserToReader(context.Background())
			endAdd()
			bs, err := io.ReadAll(r)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if len(bs) != 0 {
				t.Errorf("unexpected result: want 0, got %v", len(bs))
			}
		}
	})

	t.Run("存在空流", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			data, data2 := MakeByteArray(3)
			rcs := make([]io.ReadCloser, 0, 4)
			rcs = append(rcs, NewReader(data[0], nil, nil, nil))
			rcs = append(rcs, NewReader(data[1], nil, nil, nil))
			rcs = append(rcs, nil)
			rcs = append(rcs, NewReader(data[2], nil, nil, nil))
			r, add, endAdd := iu.NewMultiReadCloserToReader(context.Background(), rcs...)
			data3, data4 := MakeByteArray(3)
			go func() {
				rcs = append(rcs[:0], NewReader(data3[0], nil, nil, nil))
				rcs = append(rcs, NewReader(data3[1], nil, nil, nil))
				rcs = append(rcs, nil)
				rcs = append(rcs, NewReader(data3[2], nil, nil, nil))
				for i := range rcs {
					err := add(rcs[i])
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
				}
				endAdd()
			}()
			data5 := append(data2, data4...)
			result, err := io.ReadAll(r)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(data5, result) {
				t.Errorf("unexpected result: want %v, got %v", len(data)+len(data2), len(result))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			data, data2 := MakeByteArray(3)
			rcs := make([]io.ReadCloser, 0, 4)
			rcs = append(rcs, NewReader(data[0], nil, nil, nil))
			rcs = append(rcs, NewReader(data[1], nil, nil, nil))
			rcs = append(rcs, NewReader(data[2], nil, nil, nil))
			ctx, cancel := NewCtxCancelWithError()
			expectedErr := errors.New("expected error")
			r, add, endAdd := iu.NewMultiReadCloserToReader(ctx, rcs...)
			wg := sync.WaitGroup{}
			wg.Add(1)
			data3, data4 := MakeByteArray(3)
			go func() {
				defer wg.Done()
				rcs = append(rcs[:0], NewReader(data3[0], nil, nil, nil))
				rcs = append(rcs, NewReader(data3[1], nil, nil, nil))
				rcs = append(rcs, NewReader(data3[2], nil, nil, nil))
				flag := rand.Intn(len(rcs))
				for i := range rcs {
					if i == flag {
						cancel(expectedErr)
					}
					err := add(rcs[i])
					if i < flag && err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					} else if err != nil && !errors.Is(err, expectedErr) {
						t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
					}
				}
				endAdd()
			}()
			data5 := append(data2, data4...)
			bs, err := io.ReadAll(r)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.HasPrefix(data5, bs) {
				t.Errorf("unexpected result: want %v, got %v", len(data)+len(data2), len(bs))
			}
			wg.Wait()
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("并发 add，上下文终止", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			data, _ := MakeByteArray(3)
			rcs := make([]io.ReadCloser, 0, 4)
			rcs = append(rcs, NewReader(data[0], nil, nil, nil))
			rcs = append(rcs, NewReader(data[1], nil, nil, nil))
			rcs = append(rcs, NewReader(data[2], nil, nil, nil))
			ctx, cancel := NewCtxCancelWithError()
			expectedErr := errors.New("expected error")
			r, add, endAdd := iu.NewMultiReadCloserToReader(ctx, rcs...)
			wg := sync.WaitGroup{}
			wg.Add(3)
			data3, _ := MakeByteArray(3)
			go func() {
				rcs = append(rcs[:0], NewReader(data3[0], nil, nil, nil))
				rcs = append(rcs, NewReader(data3[1], nil, nil, nil))
				rcs = append(rcs, NewReader(data3[2], nil, nil, nil))
				flag := rand.Intn(len(rcs))
				for i, v := range rcs {
					go func(i int, rc io.ReadCloser) {
						defer wg.Done()
						if i == flag {
							cancel(expectedErr)
						}
						err := add(rc)
						if err != nil && !errors.Is(err, expectedErr) {
							t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
						}
					}(i, v)
				}
				wg.Wait()
				endAdd()
			}()
			_, err := io.ReadAll(r)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			wg.Wait()
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("endAdd 后 add", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			data, data2 := MakeByteArray(3)
			rcs := make([]io.ReadCloser, 0, 4)
			rcs = append(rcs, NewReader(data[0], nil, nil, nil))
			rcs = append(rcs, NewReader(data[1], nil, nil, nil))
			rcs = append(rcs, NewReader(data[2], nil, nil, nil))
			r, add, endAdd := iu.NewMultiReadCloserToReader(context.Background(), rcs...)
			flag := rand.Intn(3)
			data3, data4 := MakeByteArray(3)
			go func() {
				rcs = append(rcs[:0], NewReader(data3[0], nil, nil, nil))
				rcs = append(rcs, NewReader(data3[1], nil, nil, nil))
				rcs = append(rcs, NewReader(data3[2], nil, nil, nil))
				for i := range rcs {
					if i == flag {
						endAdd()
					}
					func() {
						defer func() {
							p := recover()
							if i >= flag && p != iu.ErrAddAfterEnd {
								t.Errorf("unexpected error: want %v, got %v", iu.ErrAddAfterEnd, p)
							}
						}()
						err := add(rcs[i])
						if i < flag && err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
					}()
				}
				endAdd()
			}()
			data5 := append(data2, data4...)
			bs, err := io.ReadAll(r)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.HasPrefix(data5, bs) {
				t.Errorf("unexpected result: want %v, got %v", len(data)+len(data2), len(bs))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); int(closeCount)-3+flag != 0 {
				t.Errorf("unexpected result: want 0, got %v", int(closeCount)-3+flag)
			}
		}
	})

	t.Run("读取失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			data, data2 := MakeByteArray(3)
			rcs := make([]io.ReadCloser, 0, 4)
			rcs = append(rcs, NewReader(data[0], nil, nil, nil))
			rcs = append(rcs, NewReader(data[1], nil, nil, nil))
			rcs = append(rcs, NewReader(data[2], nil, nil, nil))
			r, add, endAdd := iu.NewMultiReadCloserToReader(context.Background(), rcs...)
			expectedErr := errors.New("expected error")
			wg := sync.WaitGroup{}
			wg.Add(1)
			data3, data4 := MakeByteArray(3)
			go func() {
				defer wg.Done()
				rcs = append(rcs[:0], NewReader(data3[0], nil, nil, nil))
				rcs = append(rcs, NewReader(data3[1], nil, expectedErr, nil))
				rcs = append(rcs, NewReader(data3[2], nil, nil, nil))
				for i := range rcs {
					err := add(rcs[i])
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
				}
				endAdd()
			}()
			data5 := append(data2, data4...)
			bs, err := io.ReadAll(r)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if !bytes.HasPrefix(data5, bs) {
				t.Errorf("unexpected result: want %v, got %v", len(data)+len(data2), len(bs))
			}
			wg.Wait()
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("关闭读取失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			data, data2 := MakeByteArray(3)
			rcs := make([]io.ReadCloser, 0, 4)
			rcs = append(rcs, NewReader(data[0], nil, nil, nil))
			rcs = append(rcs, NewReader(data[1], nil, nil, nil))
			rcs = append(rcs, NewReader(data[2], nil, nil, nil))
			r, add, endAdd := iu.NewMultiReadCloserToReader(context.Background(), rcs...)
			expectedErr := errors.New("expected error")
			wg := sync.WaitGroup{}
			wg.Add(1)
			data3, data4 := MakeByteArray(3)
			go func() {
				defer wg.Done()
				rcs = append(rcs[:0], NewReader(data3[0], nil, nil, nil))
				rcs = append(rcs, NewReader(data3[1], nil, expectedErr, nil))
				rcs = append(rcs, NewReader(data3[2], nil, nil, nil))
				for i := range rcs {
					err := add(rcs[i])
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
				}
				endAdd()
			}()
			data5 := append(data2, data4...)
			bs, err := io.ReadAll(r)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if !bytes.HasPrefix(data5, bs) {
				t.Errorf("unexpected result: want %v, got %v", len(data)+len(data2), len(bs))
			}
			wg.Wait()
			if atomic.LoadInt32(&CloseCount) != 0 {
				t.Errorf("unexpected close count: want 0, got %v", CloseCount)
			}
		}
	})

	t.Run("大量数据", func(t *testing.T) {
		atomic.StoreInt32(&CloseCount, 0)
		data := MakeBytes(1024*1024*100*(rand.Intn(5)+1) + 13)
		parts := Split(data)
		sort.Slice(parts, func(i, j int) bool { return parts[i].Offset < parts[j].Offset })
		r, add, endAdd := iu.NewMultiReadCloserToReader(context.Background())
		go func() {
			for _, v := range parts {
				err := add(NewReader(data[v.Offset:v.End], nil, nil, nil))
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
			}
			endAdd()
		}()
		bs, err := io.ReadAll(r)
		if err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if !bytes.Equal(bs, data) {
			t.Errorf("unexpected result: want %v, got %v", len(bs), len(data))
		}
		if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
			t.Errorf("unexpected close count: want 0, got %v", closeCount)
		}
	})
}
