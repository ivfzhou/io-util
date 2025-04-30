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
	"fmt"
	"io"
	"sync"
)

// NewMultiReadCloserToWriter 依次从 Reader 读出数据并写入 Writer 中，并 Close Reader。
//
// 返回 send 用于添加 Reader，readSize 表示需要从 Reader 读出的字节数，offset 用于表示记录读取序数并传递给 writer，若读取字节数对不上则返回 error。
// 如果 readSize 是 0 或者 reader 是 nil，将不会转递到 writer。
//
// 返回 wait 用于等待所有 Reader 读完，若读取发生 error，wait 返回该 error，并结束读取。
//
// 务必等所有 Reader 都已添加给 send 后再调用 wait。
//
// 该函数可用于需要非同一时间多个读取流和一个写入流的工作模型。
func NewMultiReadCloserToWriter(ctx context.Context, writer func(offset int, p []byte)) (
	send func(readSize, offset int, reader io.ReadCloser), wait func() error) {

	ctx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	errOnce := &sync.Once{}
	waitOnce := &sync.Once{}
	wg := &sync.WaitGroup{}
	var (
		returnRrr error
		cancelErr error
	)
	send = func(readSize, order int, reader io.ReadCloser) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				if cancelErr == nil {
					cancelErr = ctx.Err()
				}
				return
			default:
			}
			p := make([]byte, readSize)
			n, err := io.ReadFull(reader, p)
			if err != nil || n != len(p) {
				cancel()
				errOnce.Do(func() {
					errCh <- fmt.Errorf("reading bytes occur error, want read size %d, actual size: %d, error is %v", len(p), n, err)
					close(errCh)
				})
			}
			closeIO(reader)
			writer(order, p)
		}()
	}
	wait = func() error {
		waitOnce.Do(func() {
			wg.Wait()
			cancel()
			select {
			case returnRrr = <-errCh:
			default:
				errOnce.Do(func() { close(errCh) })
			}
		})
		if returnRrr != nil {
			return returnRrr
		}
		return cancelErr
	}

	return
}
