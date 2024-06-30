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
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

type WriteAtCloser interface {
	io.WriterAt
	// Closer
	// Deprecated: 使用CloseByError代替
	io.Closer
	CloseByError(error) error
}

type writeAtReader struct {
	tmpFile     *os.File
	writerDone  chan struct{}
	writerErr   error
	readerErr   error
	cursor      int64
	axisMarker  AxisMarker
	notify      chan struct{}
	readerClose int32
	writerClose int32
}

type writeCloser struct {
	*writeAtReader
}

type readCloser struct {
	*writeAtReader
}

type multiReader struct {
	err     error
	ctx     context.Context
	rcQueue *Queue[io.ReadCloser]
	curRc   io.ReadCloser
	lock    sync.Mutex
}

// NewWriteAtReader 获取一个WriterAt和Reader对象，其中WriterAt用于并发写入数据，而与此同时Reader对象同时读取出已经写入好的数据。
//
// WriterAt写入完毕后调用Close，则Reader会全部读取完后结束读取。
//
// WriterAt发生的error会传递给Reader返回。
//
// 该接口是特定为一个目的实现————服务器分片下载数据中转给客户端下载，提高中转数据效率。
func NewWriteAtReader() (WriteAtCloser, io.ReadCloser) {
	temp, err := os.CreateTemp("", "ivfzhou_gotools_WriteAtReader")
	if err != nil {
		panic(err)
	}
	wr := &writeAtReader{
		tmpFile:    temp,
		writerDone: make(chan struct{}),
		notify:     make(chan struct{}, 1),
	}
	return &writeCloser{wr}, &readCloser{wr}
}

// NewMultiReadCloserToReader 依次从rc中读出数据直到io.EOF则close rc。从r获取rc中读出的数据。
//
// add添加rc，返回error表明读取rc发生错误，可以安全的添加nil。调用endAdd表明不会再有rc添加，当所有数据读完了时，r将返回EOF。
//
// 如果ctx被cancel，将停止读取并返回error。
//
// 所有添加进去的io.ReadCloser都会被close。
func NewMultiReadCloserToReader(ctx context.Context, rc ...io.ReadCloser) (
	r io.Reader, add func(rc io.ReadCloser) error, endAdd func()) {

	mr := &multiReader{
		ctx:     ctx,
		rcQueue: &Queue[io.ReadCloser]{},
	}
	for i := range rc {
		if rc[i] == nil {
			continue
		}
		mr.rcQueue.Push(rc[i])
	}
	return mr,
		func(rc io.ReadCloser) error {
			if rc == nil {
				return nil
			}
			select {
			case <-mr.ctx.Done():
				closeIO(rc)
				mr.rcQueue.Close()
				if mr.err != nil {
					return mr.err
				}
				if err := mr.ctx.Err(); err != nil {
					return err
				}
				return context.Canceled
			default:
				if !mr.rcQueue.Push(rc) {
					closeIO(rc)
					if mr.err != nil {
						return mr.err
					}
					return errors.New("endAdd was called")
				}
				return mr.err
			}
		},
		func() { mr.rcQueue.Close() }
}

// NewMultiReadCloserToWriter 依次从reader读出数据并写入writer中，并close reader。
//
// 返回send用于添加reader，readSize表示需要从reader读出的字节数，order用于表示记录读取序数并传递给writer，若读取字节数对不上则返回error。
//
// 返回wait用于等待所有reader读完，若读取发生error，wait返回该error，并结束读取。
//
// 务必等所有reader都已添加给send后再调用wait。
//
// 该函数可用于需要非同一时间多个读取流和一个写入流的工作模型。
func NewMultiReadCloserToWriter(ctx context.Context, writer func(order int, p []byte)) (
	send func(readSize, order int, reader io.ReadCloser), wait func() error) {

	ctx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	errOnce := &sync.Once{}
	waitOnce := &sync.Once{}
	wg := &sync.WaitGroup{}
	var (
		returnRrr error
		cancelErr error
	)
	return func(readSize, order int, reader io.ReadCloser) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case <-ctx.Done():
					if cancelErr == nil {
						cancelErr = ctx.Err()
					}
					return
				default:
				}
				p := make([]byte, readSize)
				n, err := io.ReadFull(reader, p)
				if err != nil || n != len(p) {
					cancel()
					errOnce.Do(func() {
						errCh <- fmt.Errorf("reading bytes occur error, want read size %d, actual size: %d, error is %v", len(p), n, err)
						close(errCh)
					})
				}
				closeIO(reader)
				writer(order, p)
			}()
		},
		func() error {
			waitOnce.Do(func() {
				wg.Wait()
				cancel()
				select {
				case returnRrr = <-errCh:
				default:
					errOnce.Do(func() { close(errCh) })
				}
			})
			if returnRrr != nil {
				return returnRrr
			}
			return cancelErr
		}
}

// CopyFile 复制文件。
func CopyFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer closeIO(srcFile)

	_ = os.Mkdir(filepath.Dir(dest), 0755)
	destFile, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer closeIO(destFile)

	_, err = io.Copy(destFile, srcFile)
	return err
}

func closeIO(c io.Closer) {
	if c != nil {
		err := c.Close()
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
		}
	}
}

func (c *writeCloser) Close() error {
	if atomic.CompareAndSwapInt32(&c.writerClose, 0, 1) {
		close(c.writerDone)
		return nil
	}
	return errors.New("reader already closed")
}

func (c *writeCloser) CloseByError(err error) error {
	if atomic.CompareAndSwapInt32(&c.writerClose, 0, 1) {
		c.writerErr = err
		close(c.writerDone)
		return nil
	}
	return errors.New("reader already closed")
}

func (c *readCloser) Close() error {
	if atomic.CompareAndSwapInt32(&c.readerClose, 0, 1) {
		name := c.tmpFile.Name()
		err := c.tmpFile.Close()
		_ = os.Remove(name)
		return err
	}
	return errors.New("reader already closed")
}

func (wr *writeAtReader) WriteAt(p []byte, off int64) (int, error) {
	if len(p) <= 0 {
		return 0, nil
	}

	length := len(p)
	begin := off
	for {
		if atomic.LoadInt32(&wr.writerClose) > 0 {
			return 0, errors.New("writer was closed")
		}
		if wr.writerErr != nil {
			return 0, wr.writerErr
		}
		if wr.readerErr != nil {
			return 0, wr.readerErr
		}
		l, err := wr.tmpFile.WriteAt(p, off)
		if err != nil {
			wr.writerErr = err
			return l, err
		}
		if l == len(p) {
			break
		}
		p = p[l:]
		off += int64(l)
	}
	go func() {
		wr.axisMarker.Mark(begin, int64(length))
		select {
		case wr.notify <- struct{}{}:
		default:
		}
	}()
	return length, nil
}

func (wr *writeAtReader) Read(p []byte) (int, error) {
	if len(p) <= 0 {
		return 0, nil
	}

	for {
		if atomic.LoadInt32(&wr.readerClose) > 0 {
			return 0, errors.New("reader was closed")
		}
		if wr.writerErr != nil {
			return 0, wr.writerErr
		}
		if wr.readerErr != nil {
			return 0, wr.readerErr
		}

		select {
		case <-wr.writerDone:
			return wr.tmpFile.Read(p)
		default:
		}

		l := wr.axisMarker.GetMaxMarkLine(wr.cursor)
		if l > 0 {
			pl := int64(len(p))
			if l > pl {
				l = pl
			} else {
				p = p[:l]
			}
			n, err := wr.tmpFile.Read(p)
			if err != nil && !errors.Is(err, io.EOF) {
				wr.readerErr = err
				return n, err
			}
			atomic.AddInt64(&wr.cursor, int64(n))
			return n, nil
		}

		select {
		case <-wr.notify:
		case <-wr.writerDone:
		}
	}
}

func (r *multiReader) Read(p []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	rc := r.curRc
	var ok bool
	for rc == nil {
		select {
		case <-r.ctx.Done():
			if r.err == nil {
				r.err = r.ctx.Err()
			}
			r.rcQueue.Close()
			return 0, r.err
		case rc, ok = <-r.rcQueue.GetFromChan():
			if !ok {
				r.err = io.EOF
				return 0, r.err
			}
		}
	}

	l, err := rc.Read(p)
	if errors.Is(err, io.EOF) {
		closeIO(rc)
		r.curRc = nil
		return l, nil
	}
	if err == nil {
		r.curRc = rc
		return l, nil
	}

	r.err = err
	return 0, r.err
}
