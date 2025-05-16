# 说明

IO 操作函数库

[![codecov](https://codecov.io/gh/ivfzhou/io-util/graph/badge.svg?token=QYBRAOTH5K)](https://codecov.io/gh/ivfzhou/io-util)

# 使用

```shell
go get gitee.com/ivfzhou/io-util@latest
```

```golang
// NewWriteAtToReader 获取一个 WriteAtCloser 和 io.ReadCloser 对象，其中 wc 用于并发写入数据，与此同时 rc 读取出已经写入好的数据。
//
// wc：写入流。字节临时写入到磁盘。写入完毕后关闭，则 rc 会全部读取完后返回 io.EOF。
//
// rc：读取流。
//
// wc 发生的错误会传递给 rc 返回。

func NewWriteAtToReader() (wc WriteAtCloser, rc io.ReadCloser)

// NewMultiReadCloserToWriterAt 将 rc 数据读取并写入 wa。
//
// ctx：上下文，如果终止了将终止流读写，并返回 ctx.Err()。
//
// wa：从 rc 中读取的数据将写入它。
//
// send：添加需要读取的数据流 rc。offset 表示从 wa 指定位置开始读取。所有 rc 都将关闭。若 rc 是空，将触发恐慌。
//
// wait：添加完所有 rc 后调用，等待所有数据处理完毕。fastExit 表示当发生错误时，立刻返回该错误。
//
// 注意：若 wa 是空，将触发恐慌。
func NewMultiReadCloserToWriterAt(ctx context.Context, wa io.WriterAt) (
    send func (rc io.ReadCloser, offset int64) error, wait func (fastExit bool) error)

// NewMultiReadCloserToReader 依次将 rc 中的数据转到 r 中读出。每一个 rc 读取数据直到 io.EOF 后调用关闭。
//
// ctx：上下文。如果终止了，r 将返回 ctx.Err()。
//
// rc：要读取数据的流。可以为空。
//
// r：合并所有 rc 数据的流。
//
// add：添加 rc，返回错误表明读取 rc 发生错误，将不再读取剩余的 rc，且所有添加进去的 rc 都会调用关闭。可以安全的添加空 rc。
//
// endAdd：调用后表明不会再有 rc 添加，当所有 rc 数据读完了时，r 将返回 io.EOF。
//
// 注意：所有添加进去的 ReadCloser 都会被关闭，即使发生了错误。除非 r 没有读取直到 io.EOF。
//
// 注意：请务必调用 endAdd 以便 r 能读取完毕返回 io.EOF。
//
// 注意：在 endAdd 后再 add rc 将会触发恐慌返回 ErrAddAfterEnd，且该 rc 不会被关闭。
func NewMultiReadCloserToReader(ctx context.Context, rc ...io.ReadCloser) (
    r io.Reader, add func (rc io.ReadCloser) error, endAdd func ())
```

# 联系作者

电邮：ivfzhou@126.com  
微信：h899123
