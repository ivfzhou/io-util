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
	"errors"
	"fmt"
	"io"
	"math/rand"
	"sync/atomic"
	"time"

	iu "gitee.com/ivfzhou/io-util"
)

var CloseCount int32

type perceptionCloser struct {
	closeFlag int32
	r         io.Reader
}

type ctxCancelWithError struct {
	context.Context
	err iu.AtomicError
}

type writerAt struct {
	WriteFn func([]byte, int64) (int, error)
}

type reader struct {
	ReadFn func([]byte) (int, error)
}

type errorReader struct {
	canReadData []byte
	err         error
	closeFlag   int32
}

type errorCloseReader struct {
	canReadData []byte
	err         error
}

type readInterceptor struct {
	Fn func()
	io.ReadCloser
}

type Part struct {
	Offset, End int
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

func (c *perceptionCloser) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closeFlag, 0, 1) {
		return errors.New("closer is already closed")
	}

	atomic.AddInt32(&CloseCount, -1)
	return nil
}

func (c *perceptionCloser) Read(p []byte) (n int, err error) {
	return c.r.Read(p)
}

func (c *readInterceptor) Read(p []byte) (n int, err error) {
	c.Fn()
	return c.ReadCloser.Read(p)
}

func (c *ctxCancelWithError) Deadline() (deadline time.Time, ok bool) {
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

func (w *writerAt) WriteAt(p []byte, off int64) (n int, err error) {
	return w.WriteFn(p, off)
}

func (r *reader) Read(p []byte) (n int, err error) {
	return r.ReadFn(p)
}

func (e *errorReader) Read(p []byte) (int, error) {
	if len(e.canReadData) <= 0 {
		return 0, e.err
	}
	n := copy(p, e.canReadData)
	e.canReadData = e.canReadData[n:]
	return n, nil
}

func (e *errorReader) Close() error {
	if !atomic.CompareAndSwapInt32(&e.closeFlag, 0, 1) {
		return fmt.Errorf("reader already closed")
	}
	atomic.AddInt32(&CloseCount, -1)
	return nil
}

func (e *errorCloseReader) Read(p []byte) (int, error) {
	if len(e.canReadData) <= 0 {
		return 0, io.EOF
	}
	n := copy(p, e.canReadData)
	e.canReadData = e.canReadData[n:]
	return n, nil
}

func (e *errorCloseReader) Close() error {
	atomic.AddInt32(&CloseCount, -1)
	return e.err
}

func newErrorReader(err error, data []byte) io.ReadCloser {
	atomic.AddInt32(&CloseCount, 1)
	return &errorReader{err: err, canReadData: data}
}

func newErrorCloseReader(err error, data []byte) io.ReadCloser {
	atomic.AddInt32(&CloseCount, 1)
	return &errorCloseReader{canReadData: data, err: err}
}

func newCtxCancelWithError() (context.Context, context.CancelCauseFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &ctxCancelWithError{Context: ctx}
	return c, func(cause error) {
		c.err.Set(cause)
		cancel()
	}
}

func newClosePerception(r io.Reader) io.ReadCloser {
	atomic.AddInt32(&CloseCount, 1)
	return &perceptionCloser{r: r}
}

func newReadInterceptor(f func(), r io.ReadCloser) io.ReadCloser {
	return &readInterceptor{Fn: f, ReadCloser: r}
}
