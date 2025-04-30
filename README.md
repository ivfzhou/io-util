# 说明

IO 操作函数库

# 使用

```shell
go get gitee.com/ivfzhou/io-util@latest
```

```golang
// NewMultiReadCloserToReader 依次将 rc 中的数据转到 r 中读出。每一个 rc 读取数据直到 io.EOF 后调用关闭。
//
// ctx：上下文。如果终止了，将返回 ctx.Err()。
//
// rc：要读取数据的流。可以为空。
//
// r：合并所有 rc 数据的流。
//
// add：添加 rc，返回错误表明读取 rc 发生错误，将不再读取剩余的 rc，且所有添加进去的 rc 都会调用关闭。可以安全的添加空 rc。
//
// endAdd：调用后表明不会再有 rc 添加，当所有 rc 数据读完了时，r 将返回 io.EOF。
//
// 所有添加进去的 ReadCloser 都会被关闭，即使发生了错误。除非 r 没有读取直到 io.EOF。
//
// 请务必调用 endAdd 以便 r 能读取完毕返回 io.EOF。
//
// 注意：在 endAdd 后再 add rc 将会触发恐慌返回 ErrAddAfterEnd，且该 rc 不会被关闭。
func NewMultiReadCloserToReader(ctx context.Context, rc ...io.ReadCloser) (r io.Reader, add func(rc io.ReadCloser) error, endAdd func())

// NewWriteAtReader 获取一个WriterAt和Reader对象，其中WriterAt用于并发写入数据，而与此同时Reader对象同时读取出已经写入好的数据。
//
// WriterAt写入完毕后调用Close，则Reader会全部读取完后结束读取。
//
// WriterAt发生的error会传递给Reader返回。
//
// 该接口是特定为一个目的实现————服务器分片下载数据中转给客户端下载，提高中转数据效率。
//
// Deprecated: 使用 NewWriteAtToReader 代替。
func NewWriteAtReader() (WriteAtCloser, io.ReadCloser)
```

# 联系作者

电邮：ivfzhou@126.com  
微信：h899123
