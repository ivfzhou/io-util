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
	"context"
	crand "crypto/rand"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"sync/atomic"
	"time"

	iu "gitee.com/ivfzhou/io-util"
)

var CloseCount int32

type Part struct {
	Offset, End int
}

type ctxCancelWithError struct {
	context.Context
	err iu.AtomicError
}

type writeAtFunc struct {
	w func([]byte, int64) (int, error)
}

type readCloser2 struct {
	closeErr    error
	readErr     error
	closeFlag   int32
	data        []byte
	total       int
	readCount   int
	interceptor func()
}

type readCloser struct {
	closeErr    error
	readErr     error
	closeFlag   int32
	data        []byte
	readCount   int
	total       int
	interceptor func()
}

func Split[T any](arr []T) []*Part {
	parts := make([]*Part, 0, len(arr)/2)

	var fn func(sub []T, offset int)
	fn = func(sub []T, offset int) {
		if len(sub) <= 0 {
			return
		}
		start := rand.Intn(len(sub))
		var length int
		if len(sub[start:]) <= 0 {
			length = 0
		} else {
			length = rand.Intn(len(sub[start:]) + 1)
		}
		if length > 0 {
			parts = append(parts, &Part{offset + start, offset + start + length})
		}
		fn(sub[:start], offset)
		fn(sub[start+length:], start+length+offset)
	}

	fn(arr, 0)
	return parts
}

func MakeBytes(n int) []byte {
	if n <= 0 {
		n = 1024*1024*(rand.Intn(5)+1) + rand.Intn(14)
	}
	data := make([]byte, n)
	n, err := crand.Read(data)
	if err != nil || n != len(data) {
		panic("rand.Read fail")
	}
	return data
}

func NewWriteAt(f func([]byte, int64) (int, error)) io.WriterAt {
	return &writeAtFunc{w: f}
}

func MakeByteArray(l int) ([][]byte, []byte) {
	data := MakeBytes(0)
	arr := make([][]byte, 0, l)
	indexes := make([]int, l)
	for i := 0; i < l-1; i++ {
		indexes[i] = rand.Intn(len(data) + 1)
	}
	indexes[l-1] = len(data)
	sort.Ints(indexes)
	prevIndex := 0
	for _, v := range indexes {
		arr = append(arr, data[prevIndex:v])
		prevIndex = v
	}
	return arr, data
}

func NewCtxCancelWithError() (context.Context, context.CancelCauseFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &ctxCancelWithError{Context: ctx}
	return c, func(cause error) {
		c.err.Set(cause)
		cancel()
	}
}

func NewReader2(data []byte, interceptor func(), closeErr, readErr error) iu.ReadCloser {
	atomic.AddInt32(&CloseCount, 1)
	return &readCloser2{
		closeErr:    closeErr,
		readErr:     readErr,
		data:        data,
		total:       len(data),
		interceptor: interceptor,
	}
}

func NewReader(data []byte, interceptor func(), closeErr, readErr error) io.ReadCloser {
	atomic.AddInt32(&CloseCount, 1)
	return &readCloser{
		closeErr:    closeErr,
		readErr:     readErr,
		data:        data,
		total:       len(data),
		interceptor: interceptor,
	}
}

func (rc *readCloser) Read(p []byte) (int, error) {
	if rc.interceptor != nil {
		rc.interceptor()
	}
	if len(rc.data) <= 0 {
		if rc.readErr != nil {
			rc.data = nil
			return 0, rc.readErr
		}
		return 0, io.EOF
	}
	if rc.readErr != nil {
		if rc.readCount >= rc.total/2 {
			rc.data = nil
			return 0, rc.readErr
		}
	}
	n := copy(p, rc.data)
	rc.data = rc.data[n:]
	rc.readCount += n
	if len(rc.data) <= 0 {
		if rc.readErr != nil {
			p[len(p)-1]++
			p = p[:len(p)-1]
			rc.data = nil
			return n - 1, rc.readErr
		}
		return n, io.EOF
	}
	return n, nil
}

func (rc *readCloser) Close() error {
	if atomic.CompareAndSwapInt32(&rc.closeFlag, 0, 1) {
		atomic.AddInt32(&CloseCount, -1)
		return rc.closeErr
	}
	return fmt.Errorf("reader already closed")
}

func (c *ctxCancelWithError) Deadline() (time.Time, bool) {
	return c.Context.Deadline()
}

func (c *ctxCancelWithError) Done() <-chan struct{} {
	return c.Context.Done()
}

func (c *ctxCancelWithError) Err() error {
	return c.err.Get()
}

func (c *ctxCancelWithError) Value(key any) any {
	return c.Context.Value(key)
}

func (w *writeAtFunc) WriteAt(p []byte, off int64) (n int, err error) {
	return w.w(p, off)
}

func (rc *readCloser2) Read() ([]byte, error) {
	if rc.interceptor != nil {
		rc.interceptor()
	}
	if len(rc.data) <= 0 {
		if rc.readErr != nil {
			return nil, rc.readErr
		}
		return nil, io.EOF
	}
	if rc.readErr != nil {
		if rc.readCount >= rc.total/2 {
			rc.data = nil
			return nil, rc.readErr
		}
	}
	n := rand.Intn(len(rc.data) + 1)
	p := make([]byte, n)
	copy(p, rc.data[:n])
	rc.data = rc.data[n:]
	rc.readCount += n
	if len(rc.data) <= 0 {
		if rc.readErr != nil {
			p = p[:len(p)-1]
			rc.data = nil
			return p, rc.readErr
		}
		return p, io.EOF
	}
	return p, nil
}

func (rc *readCloser2) Close() error {
	if atomic.CompareAndSwapInt32(&rc.closeFlag, 0, 1) {
		atomic.AddInt32(&CloseCount, -1)
		return rc.closeErr
	}
	return fmt.Errorf("reader already closed")
}
