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
	"math/rand"
	"testing"

	iu "gitee.com/ivfzhou/io-util"
)

func TestWriteAtAll(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			offset := rand.Int63n(100)
			data := MakeBytes(0)
			result := make([]byte, offset+int64(len(data)))
			wa := NewWriteAt(func(p []byte, o int64) (int, error) {
				n := rand.Intn(len(p) + 1)
				if len(p) > len(result[o:]) {
					t.Errorf("unexpected p: want %v, got %v", len(p), len(result[o:]))
				}
				copy(result[o:], p[:n])
				return n, nil
			})
			written, err := iu.WriteAtAll(wa, offset, data)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if written != int64(len(data)) {
				t.Errorf("unexpected written: want %d, got %d", len(data), written)
			}
			if !bytes.Equal(result[offset:], data) {
				t.Errorf("unexpected result: want %v, got %v", string(data), string(result[offset:]))
			}
		}
	})

	t.Run("写入失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			offset := rand.Int63n(100)
			data := MakeBytes(0)
			result := make([]byte, offset+int64(len(data)))
			expectedErr := errors.New("expected error")
			count := 0
			index := rand.Intn(len(data) + 1)
			wa := NewWriteAt(func(p []byte, o int64) (int, error) {
				n := rand.Intn(len(p) + 1)
				if len(p) > len(result[o:]) {
					t.Errorf("unexpected p: want %v, got %v", len(p), len(result[o:]))
				}
				copy(result[o:], p[:n])
				count += n
				if count >= index {
					return n, expectedErr
				}
				return n, nil
			})
			written, err := iu.WriteAtAll(wa, offset, data)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if written > int64(len(data)) {
				t.Errorf("unexpected written: want %d, got %d", len(data), written)
			}
			if !bytes.HasPrefix(data, result[offset:offset+written]) {
				t.Errorf("unexpected result: want %v, got %v", string(data), string(result[offset:]))
			}
		}
	})
}
