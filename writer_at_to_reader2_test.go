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
	"math/rand"
	"runtime"
	"sync"
	"testing"

	iu "gitee.com/ivfzhou/io-util"
)

func TestNewWriteAtToReader2(t *testing.T) {
	t.Run("没有数据", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader2()
			err := wc.Close()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			result, err := iu.ReadAll(rc)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if len(result) != 0 {
				t.Errorf("unexpected result: want 0, got %v", len(result))
			}
		}
	})

	t.Run("按顺序写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			expectedResult := make([]byte, 1024*1024*3*(rand.Intn(5)+1)+10)
			for i := range expectedResult {
				expectedResult[i] = byte(rand.Intn(256))
			}

			wc, rc := iu.NewWriteAtToReader2()

			go func() {
				const part = 1024 * 1024 * 8
				for i := 0; i < len(expectedResult); i += part {
					end := i + part
					if len(expectedResult) < end {
						end = len(expectedResult)
					}
					n, err := wc.WriteAt(expectedResult[i:end], int64(i))
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if n != end-i {
						t.Errorf("unexpected result: want %v, got %v", end-i, n)
					}
				}
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

			expectedResult = nil
			result = nil
			runtime.GC()
		}
	})

	t.Run("不按照顺序写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			expectedResult := make([]byte, 1024*1024*(rand.Intn(5)+1)+10)
			for i := range expectedResult {
				expectedResult[i] = byte(rand.Intn(256))
			}

			const part = 999 * 999 * 8
			writeInfo := make(map[int64][]byte, len(expectedResult))
			for i := 0; i < len(expectedResult); i += part {
				end := i + part
				if len(expectedResult) < end {
					end = len(expectedResult)
				}
				writeInfo[int64(i)] = expectedResult[i:end]
			}
			keys := make(map[int64]struct{}, len(expectedResult)/part)
			for i := range writeInfo {
				keys[i] = struct{}{}
			}

			wc, rc := iu.NewWriteAtToReader2()
			go func() {
				for {
					index := rand.Intn(len(keys))
					offset := int64(0)
					for offset = range keys {
						index--
						if index < 0 {
							delete(keys, offset)
							break
						}
					}
					n, err := wc.WriteAt(writeInfo[offset], offset)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if n != len(writeInfo[offset]) {
						t.Errorf("unexpected result: want %v, got %v", len(writeInfo[offset]), n)
					}
					if len(keys) <= 0 {
						if err = wc.Close(); err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						break
					}
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

			expectedResult = nil
			result = nil
			runtime.GC()
		}
	})

	t.Run("重复位置写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			expectedResult := make([]byte, 1024*10*(rand.Intn(5)+1)+10)
			for i := range expectedResult {
				expectedResult[i] = byte(rand.Intn(256))
			}

			parts := Split(expectedResult)
			offset := int64(len(expectedResult))
			parts2 := Split(expectedResult[offset:])

			wc, rc := iu.NewWriteAtToReader2()
			wg := sync.WaitGroup{}
			wg.Add(len(parts))
			for _, w := range parts {
				go func(w *Part) {
					defer wg.Done()
					data := expectedResult[w.Offset:w.End]
					n, err := wc.WriteAt(data, int64(w.Offset))
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if n != len(data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), n)
					}
				}(w)
			}
			wg.Add(len(parts2))
			for _, w := range parts2 {
				go func(w *Part) {
					defer wg.Done()
					data := expectedResult[w.Offset:w.End]
					n, err := wc.WriteAt(data, int64(w.Offset))
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if n != len(data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), n)
					}
				}(w)
			}
			go func() {
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
			if !bytes.Equal(result, expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), len(result))
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}

			expectedResult = nil
			parts = nil
			parts2 = nil
			result = nil
			runtime.GC()
		}
	})

	t.Run("读取位置前的写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader2()
			data := []byte("hello world")
			n, err := wc.WriteAt(data, 0)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(data) {
				t.Errorf("unexpected result: want %v, got %v", len(data), n)
			}

			var readLength int64
			for {
				data, err = rc.Read()
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				readLength = int64(len(data))
				if readLength > 0 {
					break
				}
			}

			data = []byte("hello world")
			n, err = wc.WriteAt(data, readLength-1)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(data) {
				t.Errorf("unexpected result: want %v, got %v", len(data), n)
			}

			var bs []byte
			for {
				read, err := rc.Read()
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				bs = append(bs, read...)
				if len(read) <= 0 {
					break
				}
			}
			if !bytes.Equal(bs, []byte("ello world")) {
				t.Errorf("unexpected result: want %v, got %v", []byte("ello world"), string(bs))
			}

			n, err = wc.WriteAt(data, 0)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(data) {
				t.Errorf("unexpected result: want %v, got %v", len(data), n)
			}
			if err = wc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}

			bs, err = iu.ReadAll(rc)
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

	t.Run("写入位置为负数", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader2()
			_, err := wc.WriteAt([]byte("hello world"), -1)
			if !errors.Is(err, iu.ErrOffsetCannotNegative) {
				t.Errorf("unexpected error: want %v, got %v", iu.ErrOffsetCannotNegative, err)
			}
			_ = rc.Close()
			_ = wc.Close()
		}
	})

	t.Run("写入数据为空", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader2()
			n, err := wc.WriteAt([]byte(""), 0)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 0 {
				t.Errorf("unexpected result: want 0, got %v", n)
			}
			_ = rc.Close()
			_ = wc.Close()
		}
	})

	t.Run("wc 关闭写入后，再写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader2()
			err := wc.Close()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			n, err := wc.WriteAt([]byte("hello world"), 0)
			if !errors.Is(err, iu.ErrWriterIsClosed) {
				t.Errorf("unexpected error: want %v, got %v", iu.ErrWriterIsClosed, err)
			}
			if n != 0 {
				t.Errorf("unexpected result: want 0, got %v", n)
			}
			_ = rc.Close()
		}
	})

	t.Run("rc 关闭后，再写入", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader2()
			err := rc.Close()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			expectedResult := []byte("hello world")
			n, err := wc.WriteAt(expectedResult, 0)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(expectedResult) {
				t.Errorf("unexpected result: want %v, got %v", len(expectedResult), n)
			}
			_ = wc.Close()
		}
	})

	t.Run("rc 关闭后，再读取", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader2()
			err := rc.Close()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			n, err := rc.Read()
			if !errors.Is(err, iu.ErrReaderIsClosed) {
				t.Errorf("unexpected error: want %v, got %v", iu.ErrReaderIsClosed, err)
			}
			if len(n) != 0 {
				t.Errorf("unexpected result: want 0, got %v", len(n))
			}
			_ = wc.Close()
		}
	})

	t.Run("读取数据为空", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			wc, rc := iu.NewWriteAtToReader2()
			n, err := rc.Read()
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if len(n) != 0 {
				t.Errorf("unexpected result: want 0, got %v", len(n))
			}
			_ = rc.Close()
			_ = wc.Close()
		}
	})

	t.Run("覆盖测试", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			data := make([]byte, 70)
			for i := range data {
				data[i] = byte(rand.Intn(256))
			}

			wc, rc := iu.NewWriteAtToReader2()
			n, err := wc.WriteAt(data[10:20], 10)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 10 {
				t.Errorf("unexpected result: want 10, got %v", n)
			}

			n, err = wc.WriteAt(data[30:40], 30)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 10 {
				t.Errorf("unexpected result: want 10, got %v", n)
			}

			n, err = wc.WriteAt(data[50:60], 50)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 10 {
				t.Errorf("unexpected result: want 10, got %v", n)
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
					n, err := wc.WriteAt(bs, offset)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if n != len(bs) {
						t.Errorf("unexpected result: want %v, got %v", len(bs), n)
					}
				}(v.offset, v.data)
			}
			wg.Wait()
			if err = wc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}

			bs, err := iu.ReadAll(rc)
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

	t.Run("覆盖测试2", func(t *testing.T) {
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
			n, err := wc.WriteAt(data[10:20], 10)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 10 {
				t.Errorf("unexpected result: want 10, got %v", n)
			}
			n, err = wc.WriteAt(data[30:40], 30)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 10 {
				t.Errorf("unexpected result: want 10, got %v", n)
			}
			n, err = wc.WriteAt(data[50:60], 50)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 10 {
				t.Errorf("unexpected result: want 10, got %v", n)
			}

			n, err = wc.WriteAt(v.data, v.offset)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(v.data) {
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

	t.Run("覆盖测试3", func(t *testing.T) {
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
			n, err := wc.WriteAt(data[10:30], 10)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 20 {
				t.Errorf("unexpected result: want 10, got %v", n)
			}
			n, err = wc.WriteAt(data[30:50], 30)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 20 {
				t.Errorf("unexpected result: want 10, got %v", n)
			}
			n, err = wc.WriteAt(data[50:60], 50)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != 10 {
				t.Errorf("unexpected result: want 10, got %v", n)
			}

			n, err = wc.WriteAt(v.data, v.offset)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(v.data) {
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
