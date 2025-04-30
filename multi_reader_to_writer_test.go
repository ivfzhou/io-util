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
	"sync"
	"testing"

	iu "gitee.com/ivfzhou/io-util"
)

type bytesReader struct {
	*bytes.Reader
	closed int
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
	send, wait := iu.NewMultiReadCloserToWriter(ctx, writer)
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

func (r *bytesReader) Close() error { r.closed++; return nil }
