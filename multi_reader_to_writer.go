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
	"io"
	"sync"
	"sync/atomic"
)

// ErrCallSendAfterWait 在 NewMultiReadCloserToWriter 中，已经调用 wait 后再调用 send 将会返回。
var ErrCallSendAfterWait = errors.New("call send after wait")

// NewMultiReadCloserToWriter 依次从 reader 读出数据并写入 writer 中，并关闭 reader。
//
// ctx：上下文，如果上下文终止了，将停止读取并返回 ctx.Err()。
//
// writer：从 reader 中读取的数据将写入它。
//
// send：提交 reader，读取 readSize 大小数据。如果读取不到足够的数据将返回错误。如果 reader 是空将触发恐慌。
//
// wait：等待所有 reader 读取完毕。调用 wait 后不可再调用 send，否则触发恐慌。
func NewMultiReadCloserToWriter(ctx context.Context, writer func(offset int, p []byte)) (
	send func(readSize, offset int, reader io.ReadCloser), wait func() error) {

	innerCtx, cancel := context.WithCancel(ctx)
	var err AtomicError
	wg := &sync.WaitGroup{}
	waitFlag := int32(0)

	send = func(readSize, offset int, reader io.ReadCloser) {
		if atomic.LoadInt32(&waitFlag) > 0 {
			panic(ErrCallSendAfterWait)
		}

		if reader == nil {
			panic("reader cannot be nil")
		}

		wg.Add(1)
		go func() {
			// 关闭流。
			defer func() {
				closeIO(reader)
				wg.Done()
			}()

			// 读取流。
			buf := make([]byte, min(256, readSize))
			for {
				// 判断上下文是否终止。
				select {
				case <-innerCtx.Done():
					select {
					case <-ctx.Done():
						err.Set(ctx.Err())
					default:
					}
					return
				default:
				}

				n, e := reader.Read(buf)
				if e != nil { // 发生错误，返回函数。
					err.Set(e)
					cancel()
					return
				}

				select {
				case <-innerCtx.Done():
					select {
					case <-ctx.Done():
						err.Set(ctx.Err())
					default:
					}
					return
				default:
				}

				// 写入数据。
				tmp := buf[:min(readSize, n)]
				writer(offset, tmp)
				readSize -= n
				offset += n

				// 读取到了足够数据，返回函数。
				if readSize <= 0 {
					return
				}

				// 扩大缓冲。
				if len(tmp) == len(buf) {
					bufferSize := growBufferSize(uint64(len(buf)))
					if bufferSize > uint64(len(buf)) {
						buf = make([]byte, bufferSize)
					}
				}
			}
		}()
	}

	waitChan := make(chan struct{})
	waitChanOnce := &sync.Once{}
	wait = func() error {
		atomic.CompareAndSwapInt32(&waitFlag, 0, 1)

		waitChanOnce.Do(func() {
			go func() {
				wg.Wait()
				close(waitChan)
			}()
		})

		// 等待结束。
		select {
		case <-innerCtx.Done():
			if !err.HasSet() {
				select {
				case <-waitChan:
					err.Set(nil)
				default:
				}

				select {
				case <-ctx.Done():
					err.Set(ctx.Err())
				default:
				}
			}
		case <-waitChan:
			err.Set(nil)
		}

		cancel()
		return err.Get()
	}

	return
}

// NewMultiReadCloserToWriterAt 将 rc 数据读取并写入 wa。
//
// ctx：上下文，如果终止了将终止流读写，并返回 ctx.Err()。
//
// wa：从 rc 中读取的数据将写入它。
//
// send：添加需要读取的数据流 rc。offset 表示从 wa 指定位置开始读取。所有 rc 都将关闭。若 rc 是空，将触发恐慌。
//
// wait：添加完所有 rc 后调用，等待所有数据处理完毕。fastExit 表示当发生错误时，立刻返回该错误。
//
// 注意：若 wa 是空，将触发恐慌。
func NewMultiReadCloserToWriterAt(ctx context.Context, wa io.WriterAt) (
	send func(rc io.ReadCloser, offset int64) error, wait func(fastExit bool) error) {

	if wa == nil {
		panic("writer cannot be nil")
	}

	innerCtx, cancel := context.WithCancel(ctx)
	wg := &sync.WaitGroup{}
	var err AtomicError
	waitFlag := int32(0)

	send = func(rc io.ReadCloser, offset int64) error {
		if atomic.LoadInt32(&waitFlag) > 0 {
			panic(ErrCallSendAfterWait)
		}

		if rc == nil {
			panic("rc cannot be nil")
		}

		wg.Add(1)
		go func() {
			defer func() {
				if e := rc.Close(); e != nil {
					err.Set(e)
					cancel()
				}
				wg.Done()
			}()

			buf := make([]byte, 256)
			n, l := 0, 0
			var e error
			for {
				// 上下文是否终止。
				select {
				case <-innerCtx.Done():
					select {
					case <-ctx.Done():
						err.Set(ctx.Err())
					default:
					}
					return
				default:
				}

				// 读取流。
				n, e = rc.Read(buf)
				if errors.Is(e, io.EOF) { // 读完了，可以返沪函数。
					return
				} else if e != nil { // 发生错误，停止读取。
					err.Set(e)
					cancel()
					return
				} else if n <= 0 {
					continue
				}

				// 写入流。
				tmp := buf[:n]
				for len(tmp) > 0 {
					l, e = wa.WriteAt(tmp, offset)
					if e != nil {
						err.Set(e)
						cancel()
						return
					}
					offset += int64(l)
					tmp = tmp[l:]

					// 上下文是否终止，避免此处死循环。
					select {
					case <-innerCtx.Done():
						select {
						case <-ctx.Done():
							err.Set(ctx.Err())
						default:
						}
						return
					default:
					}
				}

				// 扩大缓冲。
				if n == len(buf) {
					bufferSize := growBufferSize(uint64(len(buf)))
					if bufferSize > uint64(len(buf)) {
						buf = make([]byte, bufferSize)
					}
				}
			}
		}()

		return err.Get()
	}

	fastExitChan := make(chan struct{})
	fastExitChanOnce := sync.Once{}
	wait = func(fastExit bool) error {
		atomic.CompareAndSwapInt32(&waitFlag, 0, 1)

		fastExitChanOnce.Do(func() {
			go func() {
				wg.Wait()
				close(fastExitChan)
			}()
		})

		// 快速退出，获取到错误就返回。
		if fastExit {
			select {
			case <-innerCtx.Done():
				if !err.HasSet() {
					select {
					case <-fastExitChan: // 刚好 ctx 终止了，同时也 wg.Wait() 了。
						err.Set(nil)
						return err.Get()
					default:
					}

					select {
					case <-ctx.Done():
						err.Set(ctx.Err())
					default:
					}
				}
				return err.Get()
			case <-fastExitChan:
				err.Set(nil)
			}
		}

		wg.Wait()
		err.Set(nil)
		cancel()
		return err.Get()
	}

	return
}
