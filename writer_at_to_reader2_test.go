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

func ExampleNewWriteAtToReader2() {
	wc, rc := iu.NewWriteAtToReader2()

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
	bs, err := iu.ReadAll(rc)

	// 处理 bs 和 err。
	_, _ = bs, err
}

func TestNewWriteAtToReader2(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader2()
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
			result, err := iu.ReadAll(rc)
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
			expectedResult := MakeBytes(1024 * 10)
			confusedData := MakeBytes(len(expectedResult) / 4)
			wc, rc := iu.NewWriteAtToReader2()
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
				wg := sync.WaitGroup{}
				end := begin + rand.Intn(len(expectedResult)-len(expectedResult)/2)
				parts = append(parts, Split(expectedResult[begin:end])...)
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
			result, err := iu.ReadAll(rc)
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
			wc, rc := iu.NewWriteAtToReader2()
			err := wc.Close()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			bs, err := iu.ReadAll(rc)
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
		for i := 0; i < 1; i++ {
			wc, rc := iu.NewWriteAtToReader2()
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
				result, err = iu.ReadAll(rc)
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
			wc, rc := iu.NewWriteAtToReader2()
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
			wc, rc := iu.NewWriteAtToReader2()
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
			wc, rc := iu.NewWriteAtToReader2()
			err := rc.Close()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			bs, err := rc.Read()
			if !errors.Is(err, iu.ErrReaderIsClosed) {
				t.Errorf("unexpected error: want %v, got %v", iu.ErrReaderIsClosed, err)
			}
			if len(bs) != 0 {
				t.Errorf("unexpected result: want 0, got %v", len(bs))
			}
			if err = wc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
		}
	})

	t.Run("多次关闭写入流", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader2()
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
			wc, rc := iu.NewWriteAtToReader2()
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
			wc, rc := iu.NewWriteAtToReader2()
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
			result, err := iu.ReadAll(rc)
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

	t.Run("覆盖代码测试", func(t *testing.T) {
		data := MakeBytes(70)
		wc, rc := iu.NewWriteAtToReader2()
		n, err := iu.WriteAtAll(wc, 10, data[10:20])
		if err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
			return
		}
		if n != 10 {
			t.Errorf("unexpected result: want 10, got %v", n)
			return
		}
		n, err = iu.WriteAtAll(wc, 10, data[30:40])
		if err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
			return
		}
		if n != 10 {
			t.Errorf("unexpected result: want 10, got %v", n)
			return
		}
		n, err = iu.WriteAtAll(wc, 10, data[50:60])
		if err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
			return
		}
		if n != 10 {
			t.Errorf("unexpected result: want 10, got %v", n)
			return
		}
		writeInfo := []*struct {
			offset int64
			data   []byte
		}{
			{0, data[:9]},
			{0, data[:10]},
			{0, data[:11]},
			{0, data[:20]},
			{0, data[:21]},
			{0, data[:30]},
			{0, data[:31]},
			{0, data[:40]},
			{0, data[:41]},
			{0, data[:50]},
			{0, data[:51]},
			{0, data[:60]},
			{0, data},
			{9, data[9:10]},
			{9, data[9:11]},
			{9, data[9:20]},
			{9, data[9:21]},
			{9, data[9:30]},
			{9, data[9:31]},
			{9, data[9:40]},
			{9, data[9:41]},
			{9, data[9:50]},
			{9, data[9:51]},
			{9, data[9:60]},
			{9, data[9:]},
			{10, data[10:11]},
			{10, data[10:20]},
			{10, data[10:21]},
			{10, data[10:30]},
			{10, data[10:31]},
			{10, data[10:40]},
			{10, data[10:41]},
			{10, data[10:50]},
			{10, data[10:51]},
			{10, data[10:60]},
			{10, data[10:]},
			{11, data[11:12]},
			{11, data[11:20]},
			{11, data[11:21]},
			{11, data[11:30]},
			{11, data[11:31]},
			{11, data[11:40]},
			{11, data[11:41]},
			{11, data[11:50]},
			{11, data[11:51]},
			{11, data[11:60]},
			{11, data[11:]},
			{20, data[20:21]},
			{20, data[20:30]},
			{20, data[20:31]},
			{20, data[20:40]},
			{20, data[20:41]},
			{20, data[20:50]},
			{20, data[20:51]},
			{20, data[20:60]},
			{20, data[20:]},
			{21, data[21:22]},
			{21, data[21:30]},
			{21, data[21:31]},
			{21, data[21:40]},
			{21, data[21:41]},
			{21, data[21:50]},
			{21, data[21:51]},
			{21, data[21:60]},
			{21, data[21:]},
			{30, data[30:31]},
			{30, data[30:40]},
			{30, data[30:41]},
			{30, data[30:50]},
			{30, data[30:51]},
			{30, data[30:60]},
			{30, data[30:]},
			{31, data[31:32]},
			{31, data[31:40]},
			{31, data[31:41]},
			{31, data[31:50]},
			{31, data[31:51]},
			{31, data[31:60]},
			{31, data[31:]},
			{40, data[40:41]},
			{40, data[40:50]},
			{40, data[40:51]},
			{40, data[40:60]},
			{40, data[40:]},
			{41, data[41:42]},
			{41, data[41:50]},
			{41, data[41:51]},
			{41, data[41:60]},
			{41, data[41:]},
			{50, data[50:51]},
			{50, data[50:60]},
			{50, data[50:]},
			{51, data[51:52]},
			{51, data[51:60]},
			{51, data[51:]},
			{60, data[60:61]},
			{60, data[60:]},
			{61, data[61:62]},
			{61, data[61:]},
		}
		wg := sync.WaitGroup{}
		wg.Add(len(writeInfo))
		for _, v := range writeInfo {
			go func(offset int64, bs []byte) {
				defer wg.Done()
				n, err := iu.WriteAtAll(wc, offset, bs)
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
					return
				}
				if n != int64(len(bs)) {
					t.Errorf("unexpected result: want %v, got %v", len(bs), n)
					return
				}
			}(v.offset, v.data)
		}
		wg.Wait()
		if err = wc.Close(); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
			return
		}
		bs, err := iu.ReadAll(rc)
		if err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
			return
		}
		if !bytes.Equal(bs, data) {
			t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
			return
		}
		if err = rc.Close(); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
			return
		}
	})

	t.Run("覆盖代码测试2", func(t *testing.T) {
		data := []byte("Life is a journey, enjoy every step with a smile and an open heart. 🌟")
		writeInfo := []*struct {
			offset int64
			data   []byte
		}{
			{0, data[:9]},
			{0, data[:10]},
			{0, data[:11]},
			{0, data[:20]},
			{0, data[:21]},
			{0, data[:30]},
			{0, data[:31]},
			{0, data[:40]},
			{0, data[:41]},
			{0, data[:50]},
			{0, data[:51]},
			{0, data[:60]},
			{0, data},
			{9, data[9:10]},
			{9, data[9:11]},
			{9, data[9:20]},
			{9, data[9:21]},
			{9, data[9:30]},
			{9, data[9:31]},
			{9, data[9:40]},
			{9, data[9:41]},
			{9, data[9:50]},
			{9, data[9:51]},
			{9, data[9:60]},
			{9, data[9:]},
			{10, data[10:11]},
			{10, data[10:20]},
			{10, data[10:21]},
			{10, data[10:30]},
			{10, data[10:31]},
			{10, data[10:40]},
			{10, data[10:41]},
			{10, data[10:50]},
			{10, data[10:51]},
			{10, data[10:60]},
			{10, data[10:]},
			{11, data[11:12]},
			{11, data[11:20]},
			{11, data[11:21]},
			{11, data[11:30]},
			{11, data[11:31]},
			{11, data[11:40]},
			{11, data[11:41]},
			{11, data[11:50]},
			{11, data[11:51]},
			{11, data[11:60]},
			{11, data[11:]},
			{20, data[20:21]},
			{20, data[20:30]},
			{20, data[20:31]},
			{20, data[20:40]},
			{20, data[20:41]},
			{20, data[20:50]},
			{20, data[20:51]},
			{20, data[20:60]},
			{20, data[20:]},
			{21, data[21:22]},
			{21, data[21:30]},
			{21, data[21:31]},
			{21, data[21:40]},
			{21, data[21:41]},
			{21, data[21:50]},
			{21, data[21:51]},
			{21, data[21:60]},
			{21, data[21:]},
			{30, data[30:31]},
			{30, data[30:40]},
			{30, data[30:41]},
			{30, data[30:50]},
			{30, data[30:51]},
			{30, data[30:60]},
			{30, data[30:]},
			{31, data[31:32]},
			{31, data[31:40]},
			{31, data[31:41]},
			{31, data[31:50]},
			{31, data[31:51]},
			{31, data[31:60]},
			{31, data[31:]},
			{40, data[40:41]},
			{40, data[40:50]},
			{40, data[40:51]},
			{40, data[40:60]},
			{40, data[40:]},
			{41, data[41:42]},
			{41, data[41:50]},
			{41, data[41:51]},
			{41, data[41:60]},
			{41, data[41:]},
			{50, data[50:51]},
			{50, data[50:60]},
			{50, data[50:]},
			{51, data[51:52]},
			{51, data[51:60]},
			{51, data[51:]},
			{60, data[60:61]},
			{60, data[60:]},
			{61, data[61:62]},
			{61, data[61:]},
		}
		for _, v := range writeInfo {
			wc, rc := iu.NewWriteAtToReader2()
			n, err := iu.WriteAtAll(wc, 10, data[10:20])
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
				return
			}
			if n != 10 {
				t.Errorf("unexpected result: want 10, got %v", n)
				return
			}
			n, err = iu.WriteAtAll(wc, 30, data[30:40])
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
				return
			}
			if n != 10 {
				t.Errorf("unexpected result: want 10, got %v", n)
				return
			}
			n, err = iu.WriteAtAll(wc, 50, data[50:60])
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
				return
			}
			if n != 10 {
				t.Errorf("unexpected result: want 10, got %v", n)
				return
			}
			l, err := iu.WriteAtAll(wc, v.offset, v.data)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
				return
			}
			if l != int64(len(v.data)) {
				t.Errorf("unexpected result: want %v, got %v", len(data), l)
				return
			}
			if err = wc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
				return
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
				return
			}
		}
	})

	t.Run("覆盖代码测试3", func(t *testing.T) {
		data := []byte("Life is a journey, enjoy every step with a smile and an open heart. 🌟")
		writeInfo := []*struct {
			offset int64
			data   []byte
		}{
			{0, data[:9]},
			{0, data[:10]},
			{0, data[:11]},
			{0, data[:20]},
			{0, data[:21]},
			{0, data[:30]},
			{0, data[:31]},
			{0, data[:40]},
			{0, data[:41]},
			{0, data[:50]},
			{0, data[:51]},
			{0, data[:60]},
			{0, data},
			{9, data[9:10]},
			{9, data[9:11]},
			{9, data[9:20]},
			{9, data[9:21]},
			{9, data[9:30]},
			{9, data[9:31]},
			{9, data[9:40]},
			{9, data[9:41]},
			{9, data[9:50]},
			{9, data[9:51]},
			{9, data[9:60]},
			{9, data[9:]},
			{10, data[10:11]},
			{10, data[10:20]},
			{10, data[10:21]},
			{10, data[10:30]},
			{10, data[10:31]},
			{10, data[10:40]},
			{10, data[10:41]},
			{10, data[10:50]},
			{10, data[10:51]},
			{10, data[10:60]},
			{10, data[10:]},
			{11, data[11:12]},
			{11, data[11:20]},
			{11, data[11:21]},
			{11, data[11:30]},
			{11, data[11:31]},
			{11, data[11:40]},
			{11, data[11:41]},
			{11, data[11:50]},
			{11, data[11:51]},
			{11, data[11:60]},
			{11, data[11:]},
			{20, data[20:21]},
			{20, data[20:30]},
			{20, data[20:31]},
			{20, data[20:40]},
			{20, data[20:41]},
			{20, data[20:50]},
			{20, data[20:51]},
			{20, data[20:60]},
			{20, data[20:]},
			{21, data[21:22]},
			{21, data[21:30]},
			{21, data[21:31]},
			{21, data[21:40]},
			{21, data[21:41]},
			{21, data[21:50]},
			{21, data[21:51]},
			{21, data[21:60]},
			{21, data[21:]},
			{30, data[30:31]},
			{30, data[30:40]},
			{30, data[30:41]},
			{30, data[30:50]},
			{30, data[30:51]},
			{30, data[30:60]},
			{30, data[30:]},
			{31, data[31:32]},
			{31, data[31:40]},
			{31, data[31:41]},
			{31, data[31:50]},
			{31, data[31:51]},
			{31, data[31:60]},
			{31, data[31:]},
			{40, data[40:41]},
			{40, data[40:50]},
			{40, data[40:51]},
			{40, data[40:60]},
			{40, data[40:]},
			{41, data[41:42]},
			{41, data[41:50]},
			{41, data[41:51]},
			{41, data[41:60]},
			{41, data[41:]},
			{50, data[50:51]},
			{50, data[50:60]},
			{50, data[50:]},
			{51, data[51:52]},
			{51, data[51:60]},
			{51, data[51:]},
			{60, data[60:61]},
			{60, data[60:]},
			{61, data[61:62]},
			{61, data[61:]},
		}
		for _, v := range writeInfo {
			wc, rc := iu.NewWriteAtToReader2()
			n, err := iu.WriteAtAll(wc, 10, data[10:30])
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 20 {
				t.Errorf("unexpected result: want 10, got %v", n)
			}
			n, err = iu.WriteAtAll(wc, 30, data[30:50])
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 20 {
				t.Errorf("unexpected result: want 10, got %v", n)
			}
			n, err = iu.WriteAtAll(wc, 50, data[50:60])
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 10 {
				t.Errorf("unexpected result: want 10, got %v", n)
			}
			n, err = iu.WriteAtAll(wc, v.offset, v.data)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != int64(len(v.data)) {
				t.Errorf("unexpected result: want %v, got %v", len(data), n)
			}
			if err = wc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
		}
	})
}

func TestToReader(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			data := MakeBytes(0)
			rc := iu.ToReader(NewReader2(data, nil, nil, nil))
			bs, err := io.ReadAll(rc)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(bs, data) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
		}
	})

	t.Run("读取失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			data := MakeBytes(0)
			expectedErr := errors.New("expected error")
			rc := iu.ToReader(NewReader2(data, nil, nil, expectedErr))
			bs, err := io.ReadAll(rc)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if !bytes.HasPrefix(data, bs) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
		}
	})

	t.Run("关闭失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			data := MakeBytes(0)
			expectedErr := errors.New("expected error")
			rc := iu.ToReader(NewReader2(data, nil, expectedErr, nil))
			bs, err := io.ReadAll(rc)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(data, bs) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
			}
			if err = rc.Close(); !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
		}
	})

	t.Run("读取和关闭都失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			data := MakeBytes(0)
			closeErr := errors.New("close error")
			readErr := errors.New("read error")
			rc := iu.ToReader(NewReader2(data, nil, closeErr, readErr))
			bs, err := io.ReadAll(rc)
			if !errors.Is(err, readErr) {
				t.Errorf("unexpected error: want %v, got %v", readErr, err)
			}
			if err = rc.Close(); !errors.Is(err, closeErr) {
				t.Errorf("unexpected error: want %v, got %v", closeErr, err)
			}
			if !bytes.HasPrefix(data, bs) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
			}
		}
	})
}

func TestReadAll(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			data := MakeBytes(0)
			r := NewReader2(data, nil, nil, nil)
			bs, err := iu.ReadAll(r)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(bs, data) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
			}
		}
	})

	t.Run("读取失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			data := MakeBytes(0)
			expectedErr := errors.New("expected error")
			r := NewReader2(data, nil, nil, expectedErr)
			bs, err := iu.ReadAll(r)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if !bytes.HasPrefix(data, bs) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
			}
		}
	})
}
