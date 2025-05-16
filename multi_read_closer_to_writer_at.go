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
			n, l := 0, int64(0)
			var e error
			next := true
			for next {
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
				if errors.Is(e, io.EOF) { // 读完了，可以返回函数。
					next = false
				} else if e != nil { // 发生错误，停止读取。
					err.Set(e)
					cancel()
					next = false
				}
				if n <= 0 {
					if !next {
						return
					}
					continue
				}

				// 写入流。
				l, e = WriteAtAll(wa, offset, buf[:n])
				offset += l
				if e != nil {
					err.Set(e)
					cancel()
					return
				}

				// 扩大缓冲。
				if next && n == len(buf) {
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
