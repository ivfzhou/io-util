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
	"os"
	"path/filepath"
	"testing"

	iu "gitee.com/ivfzhou/io-util"
)

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	content := []byte("hello world")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := iu.CopyFile(src, dst); err != nil {
		t.Fatalf("unexpected error: want nil, got %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("unexpected result: source file should be removed, got err %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("unexpected result: want %q, got %q", content, got)
	}
}

func TestNewWriteAtReader(t *testing.T) {
	wc, rc := iu.NewWriteAtReader()
	data := []byte("hello deprecated alias")
	written, err := iu.WriteAtAll(wc, 0, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if written != int64(len(data)) {
		t.Fatalf("unexpected written: want %d, got %d", len(data), written)
	}
	if err = wc.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err = rc.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("unexpected result: want %q, got %q", data, got)
	}
}

func TestNewReadCounterNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("unexpected result: want panic, got none")
		}
	}()
	iu.NewReadCounter(nil)
}

func TestWriteAtAllNoProgress(t *testing.T) {
	// WriteAt 持续返回 (0, nil) 时，WriteAtAll 不应死循环，而应返回 io.ErrShortWrite。
	wa := NewWriteAt(func(p []byte, off int64) (int, error) {
		return 0, nil
	})
	written, err := iu.WriteAtAll(wa, 0, []byte("abc"))
	if !errors.Is(err, io.ErrShortWrite) {
		t.Errorf("unexpected error: want %v, got %v", io.ErrShortWrite, err)
	}
	if written != 0 {
		t.Errorf("unexpected written: want 0, got %d", written)
	}
}

func TestSegmentManagerDiscardReuse(t *testing.T) {
	// Discard 后复用 SegmentManager，不应读取到 Discard 之前残留的标记数据。
	m := &iu.SegmentManager{}

	first := bytes.Repeat([]byte("a"), 1000)
	if n, err := m.WriteAt(first, 0); err != nil || n != len(first) {
		t.Fatalf("unexpected write: n=%d err=%v", n, err)
	}
	buf := make([]byte, 500)
	if n, err := m.Read(buf); err != nil || n != len(buf) {
		t.Fatalf("unexpected read: n=%d err=%v", n, err)
	}

	m.Discard()

	second := bytes.Repeat([]byte("b"), 100)
	if n, err := m.WriteAt(second, 0); err != nil || n != len(second) {
		t.Fatalf("unexpected write: n=%d err=%v", n, err)
	}

	got := make([]byte, 1000)
	n, err := m.Read(got)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(second) {
		t.Fatalf("unexpected read length: want %d, got %d", len(second), n)
	}
	if !bytes.Equal(got[:n], second) {
		t.Errorf("unexpected result: want %q, got %q", second, got[:n])
	}
}
