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
	"io"
)

// CopyReaderToWriterAt 将 r 中数据写入 w，直到 r 读取返回 io.EOF 结束读取。
//
// r：要读取数据的流。
//
// w：数据要写入的流。
//
// offset：从 w 指定位置开始写入。
//
// nonBuffer：读取数据时，每次都分配新的内存。
//
// written：总计 w 写入数据的大小。
//
// err：异常信息。
//
// 注意：offset 为负数将触发恐慌。
func CopyReaderToWriterAt(r io.Reader, w io.WriterAt, offset int64, nonBuffer bool) (written int64, err error) {
	if offset < 0 {
		panic("offset cannot be negative")
	}

	buf := make([]byte, 256)
	n, l := 0, 0
	for {
		n, err = r.Read(buf)
		if errors.Is(err, io.EOF) { // 读取完毕了，返回函数。
			return written, nil
		} else if err != nil { // 读取数据发生错误，返回函数。
			return written, err
		} else if n <= 0 { // 没读取到数据，重新读取。
			continue
		}

		// 将读取到的数据写入 w。
		data := buf[:n]
		for len(data) > 0 {
			l, err = w.WriteAt(data, offset)
			if err != nil {
				return written + int64(l), err
			}
			offset += int64(l)
			data = data[l:]
			written += int64(l)
		}

		if nonBuffer { // 不使用缓存，则每次分配新的缓存。
			bufferSize := uint64(len(buf))
			if n == len(buf) {
				bufferSize = growBufferSize(uint64(len(buf)))
			}
			buf = make([]byte, bufferSize)
		} else { // 使用缓存时，如果读满了缓存，则分配更大的缓存。
			if n == len(buf) {
				bufferSize := growBufferSize(uint64(len(buf)))
				if bufferSize > uint64(len(buf)) {
					buf = make([]byte, bufferSize)
				}
			}
		}
	}
}
