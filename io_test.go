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
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gitee.com/ivfzhou/io-util"
)

type bytesReader struct {
	*bytes.Reader
	closed int
}

func ExampleNewWriteAtReader() {
	writeAtCloser, readCloser := io_util.NewWriteAtReader()

	// 并发写入数据
	go func() {
		_, _ = writeAtCloser.WriteAt(nil, 0)

		// 发生错误时关闭
		_ = writeAtCloser.CloseByError(nil)
	}()

	// 写完close
	_ = writeAtCloser.Close()

	// 同时读出写入的数据
	go func() {
		_, _ = readCloser.Read(nil)
	}()

	// 读完close
	_ = readCloser.Close()
}

func TestNewWriteAtReader(t *testing.T) {
	writeAtCloser, readCloser := io_util.NewWriteAtReader()
	wg := &sync.WaitGroup{}

	fileTotalSize := 1024*1024*1024*1 + rand.Intn(1024*10)
	file := make([]byte, fileTotalSize)
	for i := range file {
		file[i] = byte(rand.Intn(257))
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		fileSize := int64(len(file))
		gap := int64(1024 * 1024 * 8)
		frags := fileSize / gap
		m := make(map[int64]int64, frags+1)
		if residue := fileSize % gap; residue != 0 {
			m[frags] = residue
		}
		for i := frags - 1; i >= 0; i-- {
			m[i] = gap
		}
		defer func() {
			err := writeAtCloser.Close()
			if err != nil {
				t.Error("io: err is not nil", err)
			}
		}()
		wgi := &sync.WaitGroup{}
		for len(m) > 0 {
			off := int64(rand.Intn(int(frags + 1)))
			f, ok := m[off]
			if ok {
				delete(m, off)
				wgi.Add(1)
				go func(f, off int64) {
					defer wgi.Done()
					time.Sleep(100 * time.Millisecond)
					of := off * gap
					n, err := writeAtCloser.WriteAt(file[of:of+f], of)
					if err != nil {
						t.Error("io: err is not nil", err)
					}
					if int64(n) != f {
						t.Error("io: write len != bytes len", n, f)
					}
				}(f, off)
			}
		}
		wgi.Wait()
	}()

	wg.Add(1)
	buf := &bytes.Buffer{}
	go func() {
		defer wg.Done()
		defer func() {
			err := readCloser.Close()
			if err != nil {
				t.Error("io: err is not nil", err)
			}
		}()
		// now := time.Now()
		_, err := io.Copy(buf, readCloser)
		if err != nil {
			t.Error(err)
		}
		// t.Logf("NewWriteAtReader read speed %dbytes/s", int(float64(fileTotalSize)/time.Since(now).Seconds()))
	}()

	wg.Wait()
	res := buf.Bytes()
	l1 := len(res)
	l2 := l1 != len(file)
	if l2 {
		t.Error("io: length mot match", l1, len(file))
	}
	for i, v := range res {
		if file[i] != v {
			t.Error("io: not match", file[i], v, i)
		}
	}
}

func TestNewMultiReadCloserToReader(t *testing.T) {
	ctx := context.Background()
	length := 500*1024*1024 + rand.Intn(1025)
	data := make([]byte, length)
	for i := 0; i < length; i++ {
		data[i] = byte(rand.Intn(255))
	}
	rcs := make([]io.ReadCloser, rand.Intn(5))
	l := 1 + rand.Intn(length/2)
	residue := length - l
	begin := 0
	for i := range rcs {
		rcs[i] = &bytesReader{Reader: bytes.NewReader(data[begin : begin+l])}
		begin += l
		l = rand.Intn(residue / 2)
		residue -= l
	}
	r, add, endAdd := io_util.NewMultiReadCloserToReader(ctx, rcs...)
	go func() {
		next := true
		for next {
			rc := &bytesReader{Reader: bytes.NewReader(data[begin : begin+l])}
			begin += l
			if residue == 0 {
				next = false
			} else {
				l = 1 + rand.Intn(residue)
				residue -= l
			}
			rcs = append(rcs, rc)
			if err := add(rc); err != nil {
				t.Error("io: unexpected error", err)
			}
		}
		endAdd()
	}()
	// now := time.Now()
	bs, err := io.ReadAll(r)
	// t.Logf("reader speed %vbytes/s", int(float64(length)/time.Since(now).Seconds()))
	if err != nil {
		t.Error("io: unexpected error", err)
	}
	if len(data) != len(bs) {
		t.Error("io: bytes length not match", len(data), len(bs))
	} else if bytes.Compare(data, bs) != 0 {
		t.Error("io: bytes does not compared")
	}
	for i := range rcs {
		if rcs[i].(*bytesReader).closed <= 0 {
			t.Error("io: reader not closed", i)
		}
	}
}

func TestNewMultiReadCloserToWriter(t *testing.T) {
	readerNum := 10000
	length := 10
	m := make(map[int]struct{}, readerNum)
	lock := &sync.Mutex{}
	writer := func(order int, p []byte) {
		if len(p) != 10 {
			t.Error("io: p len not match", len(p))
		}
		s := string(p)
		if s != "0123456789" {
			t.Error("io: bytes unexpected", s)
		}
		lock.Lock()
		_, ok := m[order]
		if ok {
			t.Error("io: unexpected order", order)
		}
		m[order] = struct{}{}
		lock.Unlock()
	}
	ctx := context.Background()
	rs := make([]*bytesReader, readerNum)
	send, wait := io_util.NewMultiReadCloserToWriter(ctx, writer)
	for i := 0; i < readerNum; i++ {
		reader := &bytesReader{Reader: bytes.NewReader([]byte("0123456789"))}
		rs[i] = reader
		send(length, i, reader)
	}
	err := wait()
	if err != nil {
		t.Error("io: don't want error", err)
	}
	for i := range rs {
		if rs[i].closed != 1 {
			t.Error("io: reader doesn't close", rs[i].closed)
		}
	}
	count := len(m)
	if count != readerNum {
		t.Error("io: reader numbers not match", count)
	}
}

func TestCopyFile(t *testing.T) {
	fileSize := int64(13)
	dest := `testdata/copyfile_test`
	err := io_util.CopyFile(`testdata/copyfile`, dest)
	if err != nil {
		t.Error("if: unexpected error", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Error("if: unexpected error", err)
	}
	if info.IsDir() {
		t.Error("if: not a file")
	}
	if info.Name() != filepath.Base(dest) {
		t.Error("if: file name mistake", info.Name())
	}
	if info.Size() != fileSize {
		t.Error("if: file size mistake", info.Size())
	}
	_ = os.Remove(dest)
}

func (r *bytesReader) Close() error { r.closed++; return nil }
