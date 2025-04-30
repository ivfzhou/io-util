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
	"io"
	"math/rand"
	"testing"

	iu "gitee.com/ivfzhou/io-util"
)

func ExampleNewWriteAtReader() {
	wc, rc := iu.NewWriteAtToReader()

	// 在另一个协程中写入数据。
	go func() {
		n, err := wc.WriteAt([]byte("hello world"), 0)
		// 处理 err 和 n。
		_, _ = n, err
	}()

	// 读取出 wc 中写入的数据。
	bs, err := io.ReadAll(rc)
	_, _ = bs, err
}

func TestNewWriteAtToReader(t *testing.T) {
	t.Run("没有数据", func(t *testing.T) {
		wc, rc := iu.NewWriteAtToReader()
		err := wc.Close()
		if err != nil {
			t.Errorf("WriteAtToReader close error %v", err)
		}
		bs, err := io.ReadAll(rc)
		if err != nil {
			t.Errorf("ReadAll error %v", err)
		}
		if len(bs) != 0 {
			t.Errorf("WriteAtToReader read result %v != %v", len(bs), 0)
		}
	})

	t.Run("按顺序写入", func(t *testing.T) {
		data := make([]byte, 1024*1024*(rand.Intn(150)+1)+10)
		for i := range data {
			data[i] = byte(rand.Intn(256))
		}

		wc, rc := iu.NewWriteAtToReader()

		go func() {
			const part = 1024 * 1024 * 8
			for i := 0; i < len(data); i += part {
				end := i + part
				if len(data) < end {
					end = len(data)
				}
				n, err := wc.WriteAt(data[i:end], int64(i))
				if err != nil {
					t.Errorf("NewWriteAtToReader wc write error %v", err)
				}
				if n != end-i {
					t.Errorf("WriteAtToReader wc write error %v != %v", n, end-i)
				}
			}
			err := wc.Close()
			if err != nil {
				t.Errorf("WriteAtToReader close error %v", err)
			}
		}()

		bs, err := io.ReadAll(rc)
		if err != nil {
			t.Errorf("ReadAll error %v", err)
		}
		err = rc.Close()
		if err != nil {
			t.Errorf("WriteAtToReader rc close error %v", err)
		}
		if !bytes.Equal(bs, data) {
			t.Errorf("WriteAtToReader read result %v != %v", len(bs), len(data))
		}
	})

	t.Run("不按照顺序写入", func(t *testing.T) {
		data := make([]byte, 1024*1024*(rand.Intn(150)+1)+10)
		for i := range data {
			data[i] = byte(rand.Intn(256))
		}

		const part = 999 * 999 * 8
		datas := make(map[int64][]byte, len(data))
		for i := 0; i < len(data); i += part {
			end := i + part
			if len(data) < end {
				end = len(data)
			}
			datas[int64(i)] = data[i:end]
		}
		keys := make(map[int64]struct{}, len(data)/part)
		for i := range datas {
			keys[i] = struct{}{}
		}

		wc, rc := iu.NewWriteAtToReader()
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
				n, err := wc.WriteAt(datas[offset], offset)
				if err != nil {
					t.Errorf("WriteAtToReader write error %v", err)
				}
				if n != len(datas[offset]) {
					t.Errorf("WriteAtToReader write error %v != %v", n, datas[offset])
				}
				if len(keys) <= 0 {
					err = wc.Close()
					if err != nil {
						t.Errorf("WriteAtToReader close error %v", err)
					}
					break
				}
			}
		}()

		bs, err := io.ReadAll(rc)
		if err != nil {
			t.Errorf("ReadAll error %v", err)
		}
		err = rc.Close()
		if err != nil {
			t.Errorf("WriteAtToReader rc close error %v", err)
		}
		if !bytes.Equal(bs, data) {
			t.Errorf("WriteAtToReader read result %v != %v", len(bs), len(data))
		}
	})

	t.Run("写入位置为负数", func(t *testing.T) {
		wc, rc := iu.NewWriteAtToReader()
		_, err := wc.WriteAt([]byte("hello world"), -1)
		if !errors.Is(err, iu.ErrOffsetCannotNegative) {
			t.Errorf("WriteAtToReader write error %v", err)
		}
		_ = rc.Close()
		_ = wc.Close()
	})

	t.Run("写入数据为空", func(t *testing.T) {
		wc, rc := iu.NewWriteAtToReader()
		n, err := wc.WriteAt([]byte(""), 0)
		if err != nil {
			t.Errorf("WriteAtToReader write error %v", err)
		}
		if n != 0 {
			t.Errorf("WriteAtToReader read result %v != %v", n, 0)
		}
		_ = rc.Close()
		_ = wc.Close()
	})

	t.Run("关闭写入后，再写入", func(t *testing.T) {
		wc, rc := iu.NewWriteAtToReader()
		err := wc.Close()
		if err != nil {
			t.Errorf("WriteAtToReader close error %v", err)
		}
		n, err := wc.WriteAt([]byte("hello world"), 0)
		if !errors.Is(err, iu.ErrWriterIsClosed) {
			t.Errorf("WriteAtToReader write error %v", err)
		}
		if n != 0 {
			t.Errorf("WriteAtToReader read result %v != %v", n, 0)
		}
		_ = rc.Close()
	})

	t.Run("Reader 关闭后，再写入", func(t *testing.T) {
		wc, rc := iu.NewWriteAtToReader()
		err := rc.Close()
		if err != nil {
			t.Errorf("WriteAtToReader close error %v", err)
		}
		data := []byte("hello world")
		n, err := wc.WriteAt(data, 0)
		if err != nil {
			t.Errorf("WriteAtToReader write error %v", err)
		}
		if n != len(data) {
			t.Errorf("WriteAtToReader read result %v != %v", n, len(data))
		}
		_ = wc.Close()
	})

	t.Run("Reader 关闭后，再读取", func(t *testing.T) {
		wc, rc := iu.NewWriteAtToReader()
		err := rc.Close()
		if err != nil {
			t.Errorf("WriteAtToReader close error %v", err)
		}
		n, err := rc.Read(nil)
		if !errors.Is(err, iu.ErrReaderIsClosed) {
			t.Errorf("WriteAtToReader read error %v", err)
		}
		if n != 0 {
			t.Errorf("WriteAtToReader read result %v != %v", n, 0)
		}
		_ = wc.Close()
	})

	t.Run("读取数据为空", func(t *testing.T) {
		wc, rc := iu.NewWriteAtToReader()
		n, err := rc.Read(nil)
		if err != nil {
			t.Errorf("WriteAtToReader read error %v", err)
		}
		if n != 0 {
			t.Errorf("WriteAtToReader read result %v != %v", n, 0)
		}
		_ = rc.Close()
		_ = wc.Close()
	})
}
