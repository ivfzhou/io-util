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
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	iu "gitee.com/ivfzhou/io-util"
)

var closeCount int32

type perceptionCloser struct {
	count int
	r     io.Reader
}

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
	t.Run("正常读取", func(t *testing.T) {
		atomic.StoreInt32(&closeCount, 0)

		rc := make([]io.ReadCloser, 0, 4)
		rc = append(rc, newClosePerception(strings.NewReader("hello")))
		rc = append(rc, newClosePerception(strings.NewReader(" ")))
		rc = append(rc, newClosePerception(strings.NewReader("world. ")))

		ctx := context.Background()
		r, add, endAdd := iu.NewMultiReadCloserToReader(ctx, rc...)

		go func() {
			rc = rc[:0]
			rc = append(rc, newClosePerception(strings.NewReader("this ")))
			rc = append(rc, newClosePerception(strings.NewReader("is ")))
			rc = append(rc, newClosePerception(strings.NewReader("a ")))
			rc = append(rc, newClosePerception(strings.NewReader("test!")))
			for i := range rc {
				err := add(rc[i])
				if err != nil {
					t.Errorf("NewMultiReadCloserToReader add error %v", err)
				}
			}
			endAdd()
		}()

		bs, err := io.ReadAll(r)
		if err != nil {
			t.Errorf("NewMultiReadCloserToReader ReadAll error %v", err)
		}
		data := "hello world. this is a test!"
		if string(bs) != data {
			t.Errorf("NewMultiReadCloserToReader ReadAll got %s, want %s", string(bs), data)
		}
		if atomic.LoadInt32(&closeCount) != 0 {
			t.Errorf("NewMultiReadCloserToReader ReadAll got %d, want %d", closeCount, 0)
		}
	})

	t.Run("没有数据", func(t *testing.T) {
		atomic.StoreInt32(&closeCount, 0)

		ctx := context.Background()
		r, _, endAdd := iu.NewMultiReadCloserToReader(ctx)
		endAdd()
		bs, err := io.ReadAll(r)
		if err != nil {
			t.Errorf("NewMultiReadCloserToReader ReadAll error %v", err)
		}
		if len(bs) != 0 {
			t.Errorf("NewMultiReadCloserToReader ReadAll got %d, want %d", len(bs), 0)
		}
		if atomic.LoadInt32(&closeCount) != 0 {
			t.Errorf("NewMultiReadCloserToReader ReadAll got %d, want %d", closeCount, 0)
		}
	})

	t.Run("空流", func(t *testing.T) {
		atomic.StoreInt32(&closeCount, 0)

		rc := make([]io.ReadCloser, 0, 4)
		rc = append(rc, newClosePerception(strings.NewReader("hello")))
		rc = append(rc, newClosePerception(strings.NewReader(" ")))
		rc = append(rc, nil)
		rc = append(rc, newClosePerception(strings.NewReader("world. ")))

		ctx := context.Background()
		r, add, endAdd := iu.NewMultiReadCloserToReader(ctx, rc...)

		go func() {
			rc = rc[:0]
			rc = append(rc, newClosePerception(strings.NewReader("this ")))
			rc = append(rc, nil)
			rc = append(rc, newClosePerception(strings.NewReader("is ")))
			rc = append(rc, newClosePerception(strings.NewReader("a ")))
			rc = append(rc, newClosePerception(strings.NewReader("test!")))
			for i := range rc {
				err := add(rc[i])
				if err != nil {
					t.Errorf("NewMultiReadCloserToReader add error %v", err)
				}
			}
			endAdd()
		}()

		bs, err := io.ReadAll(r)
		if err != nil {
			t.Errorf("NewMultiReadCloserToReader ReadAll error %v", err)
		}
		data := "hello world. this is a test!"
		if string(bs) != data {
			t.Errorf("NewMultiReadCloserToReader ReadAll got %s, want %s", string(bs), data)
		}
		if atomic.LoadInt32(&closeCount) != 0 {
			t.Errorf("NewMultiReadCloserToReader ReadAll got %d, want %d", closeCount, 0)
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			atomic.StoreInt32(&closeCount, 0)

			rc := make([]io.ReadCloser, 0, 4)
			rc = append(rc, newClosePerception(strings.NewReader("hello")))
			rc = append(rc, newClosePerception(strings.NewReader(" ")))
			rc = append(rc, newClosePerception(strings.NewReader("world. ")))

			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			r, add, endAdd := iu.NewMultiReadCloserToReader(ctx, rc...)

			wg := sync.WaitGroup{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				rc = rc[:0]
				rc = append(rc, newClosePerception(strings.NewReader("this ")))
				rc = append(rc, newClosePerception(strings.NewReader("is ")))
				rc = append(rc, newClosePerception(strings.NewReader("a ")))
				rc = append(rc, newClosePerception(strings.NewReader("test!")))
				flag := rand.Intn(4)
				for i := range rc {
					if i == flag {
						cancel()
					}
					err := add(rc[i])
					if i < flag && err != nil {
						t.Errorf("NewMultiReadCloserToReader add error %v", err)
					} else if !errors.Is(err, ctx.Err()) {
						t.Errorf("NewMultiReadCloserToReader add error %v, want %v", err, ctx.Err())
					}
				}
				endAdd()
			}()

			bs, err := io.ReadAll(r)
			if !errors.Is(err, ctx.Err()) {
				t.Errorf("NewMultiReadCloserToReader ReadAll error %v", err)
			}
			data := "hello world. this is a test!"
			if len(bs) == len(data) {
				t.Errorf("NewMultiReadCloserToReader ReadAll got %s, want %s", string(bs), data)
			}
			wg.Wait()
			val := atomic.LoadInt32(&closeCount)
			if val != 0 {
				t.Errorf("NewMultiReadCloserToReader ReadAll got %d, want %d", val, 0)
			}
		}
	})

	t.Run("endAdd 后 add", func(t *testing.T) {
		atomic.StoreInt32(&closeCount, 0)

		rc := make([]io.ReadCloser, 0, 4)
		rc = append(rc, newClosePerception(strings.NewReader("hello")))
		rc = append(rc, newClosePerception(strings.NewReader(" ")))
		rc = append(rc, newClosePerception(strings.NewReader("world. ")))

		ctx := context.Background()
		r, add, endAdd := iu.NewMultiReadCloserToReader(ctx, rc...)

		flag := rand.Intn(4)
		go func() {
			rc = rc[:0]
			rc = append(rc, newClosePerception(strings.NewReader("this ")))
			rc = append(rc, newClosePerception(strings.NewReader("is ")))
			rc = append(rc, newClosePerception(strings.NewReader("a ")))
			rc = append(rc, newClosePerception(strings.NewReader("test!")))
			for i := range rc {
				if i == flag {
					endAdd()
				}
				func() {
					defer func() {
						p := recover()
						if i >= flag && p != iu.ErrAddAfterEnd {
							t.Errorf("NewMultiReadCloserToReader ReadAll error %v", p)
						}
					}()
					err := add(rc[i])
					if i < flag && err != nil {
						t.Errorf("NewMultiReadCloserToReader add error %v", err)
					}
				}()
			}
			endAdd()
		}()

		bs, err := io.ReadAll(r)
		if err != nil {
			t.Errorf("NewMultiReadCloserToReader ReadAll error %v", err)
		}
		data := "hello world. this is a test!"
		if len(bs) >= len(data) {
			t.Errorf("NewMultiReadCloserToReader ReadAll got %s, want %s", string(bs), data)
		}
		if int(atomic.LoadInt32(&closeCount))-(4-flag) != 0 {
			t.Errorf("NewMultiReadCloserToReader ReadAll got %d, want %d", closeCount, 0)
		}
	})

	t.Run("大量数据", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			atomic.StoreInt32(&closeCount, 0)

			data := make([]byte, 1024*1024*(rand.Intn(150)+1))
			for i := range data {
				data[i] = byte(rand.Intn(256))
			}

			type part struct {
				offset, length int
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
					parts = append(parts, &part{offset + start, length})
				}
				fn(arr[:start], offset)
				fn(arr[start+length:], start+length+offset)
			}
			fn(data, 0)
			sort.Slice(parts, func(i, j int) bool { return parts[i].offset < parts[j].offset })

			ctx := context.Background()
			r, add, endAdd := iu.NewMultiReadCloserToReader(ctx)

			go func() {
				for _, v := range parts {
					err := add(newClosePerception(bytes.NewReader(data[v.offset : v.offset+v.length])))
					if err != nil {
						t.Errorf("NewMultiReadCloserToReader add error %v", err)
					}
				}
				endAdd()
			}()

			bs, err := io.ReadAll(r)
			if err != nil {
				t.Errorf("NewMultiReadCloserToReader ReadAll error %v", err)
			}
			if !bytes.Equal(bs, data) {
				t.Errorf("NewMultiReadCloserToReader ReadAll got %d, want %d", len(bs), len(data))
			}
			if atomic.LoadInt32(&closeCount) != 0 {
				t.Errorf("NewMultiReadCloserToReader ReadAll got %d, want %d", closeCount, 0)
			}
		}
	})
}

func newClosePerception(r io.Reader) io.ReadCloser {
	atomic.AddInt32(&closeCount, 1)
	return &perceptionCloser{r: r}
}

func (c *perceptionCloser) Close() error {
	if c.count > 0 {
		return errors.New("closer is already closed")
	}
	c.count++
	atomic.AddInt32(&closeCount, -1)
	return nil
}

func (c *perceptionCloser) Read(p []byte) (n int, err error) {
	return c.r.Read(p)
}
