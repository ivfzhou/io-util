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
	"math"
	"testing"
)

// recordingCloser 用于记录 Close 调用情况的测试辅助类型。
type recordingCloser struct {
	err    error
	closed int
}

func (c *recordingCloser) Close() error {
	c.closed++
	return c.err
}

func TestWrapError(t *testing.T) {
	if got := WrapError(nil); got != nil {
		t.Errorf("unexpected result: want nil, got %v", got)
	}

	sentinel := errors.New("sentinel")
	got := WrapError(sentinel)
	if got == nil {
		t.Fatal("unexpected result: want non-nil, got nil")
	}
	if !errors.Is(got, sentinel) {
		t.Errorf("unexpected result: want %v, got %v", sentinel, got)
	}
}

func TestErrorMethods(t *testing.T) {
	sentinel := errors.New("some error")
	e, ok := WrapError(sentinel).(*Error)
	if !ok {
		t.Fatalf("unexpected type: want *Error, got %T", WrapError(sentinel))
	}
	if got := e.Error(); got != sentinel.Error() {
		t.Errorf("unexpected Error: want %q, got %q", sentinel.Error(), got)
	}
	if got := e.String(); got != sentinel.Error() {
		t.Errorf("unexpected String: want %q, got %q", sentinel.Error(), got)
	}
	if got := e.Unwrap(); got != sentinel {
		t.Errorf("unexpected Unwrap: want %v, got %v", sentinel, got)
	}
	if !errors.Is(e, sentinel) {
		t.Errorf("unexpected result: errors.Is should be true")
	}
}

func TestAtomicError(t *testing.T) {
	t.Run("初始状态", func(t *testing.T) {
		var e AtomicError
		if e.HasSet() {
			t.Errorf("unexpected HasSet: want false, got true")
		}
		if got := e.Get(); got != nil {
			t.Errorf("unexpected Get: want nil, got %v", got)
		}
	})

	t.Run("设置错误", func(t *testing.T) {
		var e AtomicError
		sentinel := errors.New("sentinel")
		if !e.Set(sentinel) {
			t.Errorf("unexpected Set: want true, got false")
		}
		if !e.HasSet() {
			t.Errorf("unexpected HasSet: want true, got false")
		}
		if got := e.Get(); !errors.Is(got, sentinel) {
			t.Errorf("unexpected Get: want %v, got %v", sentinel, got)
		}
	})

	t.Run("设置空错误", func(t *testing.T) {
		var e AtomicError
		if !e.Set(nil) {
			t.Errorf("unexpected Set: want true, got false")
		}
		if !e.HasSet() {
			t.Errorf("unexpected HasSet: want true, got false")
		}
		if got := e.Get(); got != nil {
			t.Errorf("unexpected Get: want nil, got %v", got)
		}
	})

	t.Run("只能设置一次", func(t *testing.T) {
		var e AtomicError
		first := errors.New("first")
		second := errors.New("second")
		if !e.Set(first) {
			t.Errorf("unexpected Set: want true, got false")
		}
		if e.Set(second) {
			t.Errorf("unexpected Set: want false, got true")
		}
		if got := e.Get(); !errors.Is(got, first) {
			t.Errorf("unexpected Get: want %v, got %v", first, got)
		}
	})
}

func TestAdditionOverflow(t *testing.T) {
	t.Run("不会溢出", func(t *testing.T) {
		additionOverflow(0, 0)
		additionOverflow(1, 1)
		additionOverflow(-1, -1)
		additionOverflow(math.MaxInt, 0)
		additionOverflow(math.MaxInt, -1)
		additionOverflow(0, math.MaxInt)
		additionOverflow(math.MinInt, 1)
	})

	t.Run("正数溢出", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Errorf("unexpected result: want panic, got none")
			}
		}()
		additionOverflow(math.MaxInt, 1)
	})

	t.Run("负数溢出", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Errorf("unexpected result: want panic, got none")
			}
		}()
		additionOverflow(math.MinInt, -1)
	})
}

func TestGrowBufferSize(t *testing.T) {
	tests := []struct {
		size uint64
		want uint64
	}{
		{1, 2},
		{2, 4},
		{3, 4},
		{4, 8},
		{255, 256},
		{256, 512},
		{257, 512},
		{300, 512},
		{511, 512},
		{512, 1024},
		{maxBufferSize - 1, maxBufferSize},
		{maxBufferSize, maxBufferSize},
		{maxBufferSize + 1, maxBufferSize},
	}
	for _, tt := range tests {
		if got := growBufferSize(tt.size); got != tt.want {
			t.Errorf("growBufferSize(%d): want %d, got %d", tt.size, tt.want, got)
		}
	}
}

func TestCloseIO(t *testing.T) {
	t.Run("关闭单个", func(t *testing.T) {
		c := &recordingCloser{}
		closeIO(c)
		if c.closed != 1 {
			t.Errorf("unexpected closed count: want 1, got %d", c.closed)
		}
	})

	t.Run("忽略空值", func(t *testing.T) {
		c := &recordingCloser{}
		closeIO(nil, c, nil)
		if c.closed != 1 {
			t.Errorf("unexpected closed count: want 1, got %d", c.closed)
		}
	})

	t.Run("关闭失败不中断", func(t *testing.T) {
		c1 := &recordingCloser{err: errors.New("close error")}
		c2 := &recordingCloser{}
		closeIO(c1, c2)
		if c1.closed != 1 {
			t.Errorf("unexpected closed count: want 1, got %d", c1.closed)
		}
		if c2.closed != 1 {
			t.Errorf("unexpected closed count: want 1, got %d", c2.closed)
		}
	})
}
