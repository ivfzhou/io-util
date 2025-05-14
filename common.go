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
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sync/atomic"
)

const maxBufferSize = 8 * 1024 * 1024

// 代表错误信息是空。
var nilError = WrapError(errors.New(""))

// Error 错误对象。
type Error struct {
	err error
}

// AtomicError 原子性读取和设置错误信息。
type AtomicError struct {
	err atomic.Value
}

// WrapError 包裹错误信息。
func WrapError(err error) error {
	if err == nil {
		return nil
	}
	return &Error{err}
}

// Set 设置错误信息，除非 err 是空。返回真表示设置成功。
func (e *AtomicError) Set(err error) bool {
	if err == nil {
		err = nilError
	} else {
		err = WrapError(err)
	}
	return e.err.CompareAndSwap(nil, err)
}

// Get 获取内部错误信息。
func (e *AtomicError) Get() error {
	err, _ := e.err.Load().(error)
	if err == nil || err == nilError {
		return nil
	}
	return err.(*Error).err
}

// HasSet 是否设置了错误信息。
func (e *AtomicError) HasSet() bool {
	err, _ := e.err.Load().(error)
	return err != nil
}

// String 接口实现。
func (e *Error) String() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

// Error 接口实现。
func (e *Error) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

// Unwrap 接口实现。
func (e *Error) Unwrap() error {
	return e.err
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

// 确保 x - y 计算不会溢出。
func substractionOverflow(x, y int) {
	if x < 0 && y > 0 {
		if x < -math.MaxInt+y {
			panic(fmt.Sprintf("calculate overflow: %d - %d", x, y))
		}
		return
	}
	if x > 0 && y < 0 {
		if x > math.MaxInt+y {
			panic(fmt.Sprintf("calculate overflow: %d - %d", x, y))
		}
	}
}

// 确保 x + y 计算不会溢出。
func additionOverflow(x, y int) {
	if x < 0 && y < 0 {
		if x < -(math.MaxInt + y) {
			panic(fmt.Sprintf("calculate overflow: %d + %d", x, y))
		}
		return
	}
	if x > 0 && y > 0 {
		if x > math.MaxInt-y {
			panic(fmt.Sprintf("calculate overflow: %d + %d", x, y))
		}
	}
}

func growBufferSize(size uint64) uint64 {
	if size >= maxBufferSize {
		return maxBufferSize
	}
	n := size - 1
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	n++
	if n == size {
		n <<= 1
	}
	return n
}
