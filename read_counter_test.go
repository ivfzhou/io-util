package io_util_test

import (
	"bytes"
	"io"
	"testing"

	iu "gitee.com/ivfzhou/io-util"
)

func TestNewReadCounter(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for range 1000 {
			data := MakeBytes(0)
			readCounter := iu.NewReadCounter(bytes.NewReader(data))
			readBytes, err := io.ReadAll(readCounter)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !bytes.Equal(readBytes, data) {
				t.Errorf("read bytes not equal: %v %v", len(readBytes), len(data))
			}
			count := readCounter.Count()
			if count != int64(len(data)) {
				t.Errorf("read count not equal: %v %v", count, len(data))
			}
		}
	})
}
