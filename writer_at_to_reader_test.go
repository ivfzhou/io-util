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
	"errors"
	"io"
	"math/rand"
	"sort"
	"sync"
	"testing"

	iu "gitee.com/ivfzhou/io-util"
)

func ExampleNewWriteAtToReader() {
	wc, rc := iu.NewWriteAtToReader()

	// 读取完后关闭流。
	defer rc.Close()

	// 在另一个协程中写入数据。
	go func() {
		// 可并发写入。
		n, err := wc.WriteAt([]byte("hello world"), 0)

		// 处理 err 和 n。
		_, _ = n, err

		// 写入完毕后关闭流。
		wc.Close()

		// 如果处理过程中发生错误，可将错误信息传递给 rc。
		wc.CloseByError(errors.New("some error"))
	}()

	// 读取出 wc 中写入的数据。
	bs, err := io.ReadAll(rc)

	// 处理 bs 和 err。
	_, _ = bs, err
}

func TestNewWriteAtToReader(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader()
			expectedResult := MakeBytes(0)
			go func() {
				var part = min(len(expectedResult)/10, 1024*1024*8)
				wg := sync.WaitGroup{}
				for i := 0; i < len(expectedResult); i += part {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						end := min(i+part, len(expectedResult))
						n, err := iu.WriteAtAll(wc, int64(i), expectedResult[i:end])
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						if n != int64(end-i) {
							t.Errorf("unexpected result: want %v, got %v", end-i, n)
						}
					}(i)
				}
				wg.Wait()
				err := wc.Close()
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
			}()
			result, err := io.ReadAll(rc)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(result, expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), len(result))
			}
		}
	})

	t.Run("同一字节位置可能多次写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			expectedResult := MakeBytes(0)
			confusedData := MakeBytes(len(expectedResult) / 4)
			wc, rc := iu.NewWriteAtToReader()
			written, err := iu.WriteAtAll(wc, rand.Int63n(int64(len(expectedResult)-len(confusedData))), confusedData)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if written != int64(len(confusedData)) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), written)
			}
			wg := sync.WaitGroup{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				parts := Split(expectedResult)
				begin := rand.Intn(len(expectedResult) / 2)
				end := begin + rand.Intn(len(expectedResult)-len(expectedResult)/2)
				parts = append(parts, Split(expectedResult[begin:end])...)
				wg := sync.WaitGroup{}
				for _, v := range parts {
					wg.Add(1)
					go func(v *Part) {
						defer wg.Done()
						n, err := iu.WriteAtAll(wc, int64(v.Offset), expectedResult[v.Offset:v.End])
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						if n != int64(v.End-v.Offset) {
							t.Errorf("unexpected result: want %v, got %v", v.End-v.Offset, n)
						}
					}(v)
				}
				wg.Wait()
				err := wc.Close()
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
			}()
			wg.Wait()
			result, err := io.ReadAll(rc)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(result, expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), len(result))
			}
		}
	})

	t.Run("没有数据写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader()
			err := wc.Close()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			bs, err := io.ReadAll(rc)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if len(bs) != 0 {
				t.Errorf("unexpected result: want 0, got %v", len(bs))
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
		}
	})

	t.Run("写入发生失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader()
			expectedErr := errors.New("expected error")
			data := MakeBytes(0)
			wg := sync.WaitGroup{}
			parts := Split(data)
			wg.Add(len(parts))
			go func() {
				occurErrIndex := rand.Intn(len(parts))
				for i, v := range parts {
					go func(i int, v *Part) {
						defer wg.Done()
						n, err := iu.WriteAtAll(wc, int64(v.Offset), data[v.Offset:v.End])
						if err != nil && !errors.Is(err, expectedErr) && !errors.Is(err, iu.ErrWriterIsClosed) {
							t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
						}
						if err == nil && n != int64(v.End-v.Offset) {
							t.Errorf("unexpected result: want %v, got %v", v.End-v.Offset, n)
						}
						if i == occurErrIndex {
							if err = wc.CloseByError(expectedErr); err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
						}
					}(i, v)
				}
			}()
			wg.Add(1)
			var result []byte
			go func() {
				defer wg.Done()
				var err error
				result, err = io.ReadAll(rc)
				if !errors.Is(err, expectedErr) {
					t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
				}
				if err = rc.Close(); err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
			}()
			wg.Wait()
			if !bytes.HasPrefix(data, result) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(result))
			}
		}
	})

	t.Run("写入位置为负数", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader()
			n, err := wc.WriteAt([]byte(""), -1)
			if !errors.Is(err, iu.ErrOffsetCannotNegative) {
				t.Errorf("unexpected error: want %v, got %v", iu.ErrOffsetCannotNegative, err)
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 0 {
				t.Errorf("unexpected result: want 0, got %v", n)
			}
			if err = wc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
		}
	})

	t.Run("关闭写入流后，再写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader()
			err := wc.Close()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			n, err := wc.WriteAt([]byte(""), 0)
			if !errors.Is(err, iu.ErrWriterIsClosed) {
				t.Errorf("unexpected error: want %v, got %v", iu.ErrWriterIsClosed, err)
			}
			if n != 0 {
				t.Errorf("unexpected result: want 0, got %v", n)
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
		}
	})

	t.Run("关闭读取流后，再读取", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader()
			err := rc.Close()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			n, err := rc.Read([]byte("xxx"))
			if !errors.Is(err, iu.ErrReaderIsClosed) {
				t.Errorf("unexpected error: want %v, got %v", iu.ErrReaderIsClosed, err)
			}
			if n != 0 {
				t.Errorf("unexpected result: want 0, got %v", n)
			}
			if err = wc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
		}
	})

	t.Run("多次关闭写入流", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader()
			err := wc.Close()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = wc.Close(); !errors.Is(err, iu.ErrWriterIsClosed) {
				t.Errorf("unexpected error: want %v, got %v", iu.ErrWriterIsClosed, err)
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
		}
	})

	t.Run("多次关闭读取流", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader()
			err := rc.Close()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = rc.Close(); !errors.Is(err, iu.ErrReaderIsClosed) {
				t.Errorf("unexpected error: want %v, got %v", iu.ErrReaderIsClosed, err)
			}
			if err = wc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
		}
	})

	t.Run("并发写入期间，关闭写入流", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader()
			expectedResult := MakeBytes(0)
			successWritten := sync.Map{}
			closeSuccessFlag := false
			wg := &sync.WaitGroup{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				var part = min(len(expectedResult)/10, 1024*1024*8)
				wg := sync.WaitGroup{}
				emitClose := rand.Intn(len(expectedResult)/part/2) + 1
				for i := 0; i < len(expectedResult); i += part {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						end := min(i+part, len(expectedResult))
						n, err := iu.WriteAtAll(wc, int64(i), expectedResult[i:end])
						if err == nil {
							successWritten.Store(i, expectedResult[i:end])
						}
						if err != nil && !errors.Is(err, iu.ErrWriterIsClosed) {
							t.Errorf("unexpected error: want %v, got %v", iu.ErrWriterIsClosed, err)
						}
						if err == nil && n != int64(end-i) {
							t.Errorf("unexpected result: want %v, got %v", end-i, n)
						}
					}(i)
					if i/part == emitClose {
						wg.Add(1)
						go func() {
							defer wg.Done()
							err := wc.Close()
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
						}()
					}
				}
				wg.Wait()
				err := wc.Close()
				if err != nil && !errors.Is(err, iu.ErrWriterIsClosed) {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				if err == nil {
					closeSuccessFlag = true
				}
			}()
			result, err := io.ReadAll(rc)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			wg.Wait()
			var offsets []int
			successWritten.Range(func(k, _ any) bool {
				offsets = append(offsets, k.(int))
				return true
			})
			sort.Ints(offsets)
			var data []byte
			prevOffset := -1
			firstOffset := -1
			for _, v := range offsets {
				if firstOffset < 0 {
					firstOffset = v
				}
				value, _ := successWritten.Load(v)
				bs := value.([]byte)
				if prevOffset >= 0 && prevOffset != v {
					break
				}
				data = append(data, bs...)
				prevOffset = v + len(bs)
			}
			if firstOffset != -1 && !closeSuccessFlag && !bytes.HasPrefix(expectedResult[firstOffset:], data) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), len(data))
			}
			if firstOffset != -1 && closeSuccessFlag && !bytes.Equal(data, expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), len(data))
			}
			if firstOffset != 0 && len(result) != 0 {
				t.Errorf("unexpected result: want 0, got %v", len(result))
			}
			if (firstOffset == 0 || firstOffset == -1) && !bytes.Equal(result, data) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(result))
			}
		}
	})
}
