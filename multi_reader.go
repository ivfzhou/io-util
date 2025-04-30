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

package io_util

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync/atomic"
)

// ErrAddAfterEnd 在 NewMultiReadCloserToReader 中，调用 endAdd 后再调用 add 将返回。
var ErrAddAfterEnd = errors.New("add after end")

// 合并流读取。
type multiReader struct {
	// 发生错误时，持有错误信息。
	err atomic.Value
	// 上下文。
	ctx context.Context
	// 读取流队列。
	readCloserQueue Queue[io.ReadCloser]
	// 当前正在读取的流。
	currentReadCloser io.ReadCloser
	// 调用 endAdd 行为标识。
	endAddSignal int32
}

// NewMultiReadCloserToReader 依次将 rc 中的数据转到 r 中读出。每一个 rc 读取数据直到 io.EOF 后调用关闭。
//
// ctx：上下文。如果终止了，将返回 ctx.Err()。
//
// rc：要读取数据的流。可以为空。
//
// r：合并所有 rc 数据的流。
//
// add：添加 rc，返回错误表明读取 rc 发生错误，将不再读取剩余的 rc，且所有添加进去的 rc 都会调用关闭。可以安全的添加空 rc。
//
// endAdd：调用后表明不会再有 rc 添加，当所有 rc 数据读完了时，r 将返回 io.EOF。
//
// 所有添加进去的 ReadCloser 都会被关闭，即使发生了错误。除非 r 没有读取直到 io.EOF。
//
// 请务必调用 endAdd 以便 r 能读取完毕返回 io.EOF。
//
// 注意：在 endAdd 后再 add rc 将会触发恐慌返回 ErrAddAfterEnd，且该 rc 不会被关闭。
func NewMultiReadCloserToReader(ctx context.Context, rc ...io.ReadCloser) (
	r io.Reader, add func(rc io.ReadCloser) error, endAdd func()) {

	// 初始化实例。
	mr := &multiReader{ctx: ctx}

	// 将 rc 添加进 Queue。
	for i := range rc {
		// 忽略 nil rc。
		if rc[i] == nil {
			continue
		}
		mr.readCloserQueue.Push(rc[i])
	}

	add = func(rc io.ReadCloser) (err error) {
		// 已调用 endAdd 不可再添加。
		if atomic.LoadInt32(&mr.endAddSignal) > 0 {
			panic(ErrAddAfterEnd)
		}

		// 忽略 nil ReadCloser。
		if rc == nil {
			err, _ = mr.err.Load().(error)
			return err
		}

		// 向队列添加 ReadCloser。
		select {
		case <-mr.ctx.Done(): // 上下文终止了，关闭队列。
			mr.readCloserQueue.Close()
			closeIO(rc)
			err, _ = mr.err.Load().(error)
			if err == nil {
				err = mr.ctx.Err()
			}
			return err
		default:
			if ok := mr.readCloserQueue.Push(rc); !ok { // 添加 ReadCloser 进队列失败，关闭 ReadCloser。
				closeIO(rc)
			}
			err, _ = mr.err.Load().(error)
			return err
		}
	}

	endAdd = func() {
		atomic.AddInt32(&mr.endAddSignal, 1) // 停止 Add，不将能再添加进去 ReadCloser。
		mr.readCloserQueue.Close()           // 关闭队列。
	}

	return mr, add, endAdd
}

func (mr *multiReader) Read(p []byte) (int, error) {
	err, _ := mr.err.Load().(error)
	if err != nil { // 已经发生了错误，就不再读取了。
		return 0, err
	}

	select {
	case <-mr.ctx.Done(): // 上下文终止了，关闭队列，关闭所有 ReadCloser，设置 err。
		mr.fail(mr.ctx.Err())
		if mr.currentReadCloser != nil { // 关闭当前正在读取的队列。
			closeIO(mr.currentReadCloser)
			mr.currentReadCloser = nil
		}
		return 0, mr.err.Load().(error)
	default:
	}

	if mr.currentReadCloser == nil { // 当前正在读取的 ReadCloser 为 nil，则从队列中获取一个。
		select {
		case <-mr.ctx.Done(): // 上下文终止了，关闭队列，关闭所有 ReadCloser，设置 err。
			mr.fail(mr.ctx.Err())
			return 0, mr.err.Load().(error)
		case rc, ok := <-mr.readCloserQueue.GetFromChan():
			if !ok { // 队列已经关闭，说明已经读取完毕了，返回 io.EOF。
				mr.err.CompareAndSwap(nil, io.EOF)
				return 0, mr.err.Load().(error)
			}
			mr.currentReadCloser = rc
		default: // 退出函数，等待下次调用。
			err, _ = mr.err.Load().(error)
			return 0, err
		}
	}

	// 读取数据。
	n, err := mr.currentReadCloser.Read(p)
	if err == nil { // 读取成功，则返回函数。
		return n, nil
	}

	if errors.Is(err, io.EOF) { // 当前 ReadCloser 读取完毕了，则关闭。
		if err = mr.currentReadCloser.Close(); err != nil { // 关闭当前 ReadCloser 失败，则关闭队列，所有所有 ReadCLoser，设置 err。
			mr.fail(err)
			return 0, mr.err.Load().(error)
		}
		mr.currentReadCloser = nil
		return n, nil
	}

	// 读取失败了，关闭所有 ReadCloser，关闭队列，设置 err。
	mr.fail(err)
	closeIO(mr.currentReadCloser)
	return 0, mr.err.Load().(error)
}

// 关闭队列，关闭队列里的所有 ReadCloser，设置 err。
func (mr *multiReader) fail(err error) {
	mr.readCloserQueue.Close()
	mr.err.CompareAndSwap(nil, err)
	for v := range mr.readCloserQueue.GetFromChan() {
		closeIO(v)
	}
}

// 关闭流。
func closeIO(closers ...io.Closer) {
	for _, closer := range closers {
		if closer != nil {
			err := closer.Close()
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "io_util", err.Error())
			}
		}
	}
}
