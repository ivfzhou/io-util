package io_util_test

import (
	"bytes"
	"errors"
	"math/rand"
	"testing"

	iu "gitee.com/ivfzhou/io-util"
)

func TestCopyReaderToWriterAt(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			data := MakeBytes(0)
			result := make([]byte, len(data))
			offset := rand.Int63n(10)
			wa := &writeAtFunc{w: func(p []byte, off int64) (int, error) {
				if len(p) > 0 {
					l := rand.Intn(len(p)) + 1
					copy(result[off-offset:], p[:l])
					return l, nil
				}
				return 0, nil
			}}
			written, err := iu.CopyReaderToWriterAt(bytes.NewReader(data), wa, offset, rand.Intn(2) == 1)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if written != int64(len(data)) {
				t.Errorf("unexpected result: want %v, got %v", len(data), written)
			}
			if !bytes.Equal(data, result) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(result))
			}
		}
	})

	t.Run("读取失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			data := MakeBytes(0)
			result := make([]byte, len(data))
			offset := rand.Int63n(10)
			wa := &writeAtFunc{w: func(p []byte, off int64) (int, error) {
				if len(p) > 0 {
					l := rand.Intn(len(p)) + 1
					copy(result[off-offset:], p[:l])
					return l, nil
				}
				return 0, nil
			}}
			expectedErr := errors.New("expected error")
			index := rand.Intn(len(data))
			written, err := iu.CopyReaderToWriterAt(NewReader(data[:index], nil, nil, expectedErr), wa, offset,
				rand.Intn(2) == 1)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if written != int64(len(data[:index])) {
				t.Errorf("unexpected result: want %v, got %v", len(data[:index]), written)
			}
			if !bytes.Equal(data[:index], result[:index]) {
				t.Errorf("unexpected result: want %v, got %v", len(data[:index]), len(result[:index]))
			}
		}
	})

	t.Run("写入失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			data := MakeBytes(0)
			result := make([]byte, len(data))
			offset := rand.Int63n(10)
			expectedErr := errors.New("expected error")
			occurErrorIndex := rand.Intn(len(data))
			writen := 0
			wa := &writeAtFunc{w: func(p []byte, off int64) (int, error) {
				if writen >= occurErrorIndex {
					return 0, expectedErr
				}
				if len(p) > 0 {
					l := rand.Intn(len(p)) + 1
					copy(result[off-offset:], p[:l])
					writen += l
					return l, nil
				}
				return 0, nil
			}}
			written, err := iu.CopyReaderToWriterAt(bytes.NewReader(data), wa, offset, rand.Intn(2) == 1)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if written < int64(occurErrorIndex) {
				t.Errorf("unexpected result: want >= %v, got %v", occurErrorIndex, written)
			}
			if !bytes.Equal(data[:writen], result[:writen]) {
				t.Errorf("unexpected result: want %v, got %v", len(data[:writen]), len(result[:writen]))
			}
		}
	})
}
