# 1. 说明

IO 操作函数库

# 2. 使用

```shell
go get gitee.com/ivfzhou/io-util@latest
```

```golang
// NewWriteAtReader 获取一个WriterAt和Reader对象，其中WriterAt用于并发写入数据，而与此同时Reader对象同时读取出已经写入好的数据。
//
// WriterAt写入完毕后调用Close，则Reader会全部读取完后结束读取。
//
// WriterAt发生的error会传递给Reader返回。
//
// 该接口是特定为一个目的实现————服务器分片下载数据中转给客户端下载，提高中转数据效率。
func NewWriteAtReader() (WriteAtCloser, io.ReadCloser)

// NewMultiReadCloserToReader 依次从rc中读出数据直到io.EOF则close rc。从r获取rc中读出的数据。
//
// add添加rc，返回error表明读取rc发生错误，可以安全的添加nil。调用endAdd表明不会再有rc添加，当所有数据读完了时，r将返回EOF。
//
// 如果ctx被cancel，将停止读取并返回error。
//
// 所有添加进去的io.ReadCloser都会被close。
func NewMultiReadCloserToReader(ctx context.Context, rc ...io.ReadCloser) (r io.Reader, add func (rc io.ReadCloser) error, endAdd func ())

// NewMultiReadCloserToWriter 依次从reader读出数据并写入writer中，并close reader。
//
// 返回send用于添加reader，readSize表示需要从reader读出的字节数，order用于表示记录读取序数并传递给writer，若读取字节数对不上则返回error。
//
// 返回wait用于等待所有reader读完，若读取发生error，wait返回该error，并结束读取。
//
// 务必等所有reader都已添加给send后再调用wait。
//
// 该函数可用于需要非同一时间多个读取流和一个写入流的工作模型。
func NewMultiReadCloserToWriter(ctx context.Context, writer func (order int, p []byte)) (send func(readSize, order int, reader io.ReadCloser), wait func () error)

// CopyFile 复制文件。
func CopyFile(src, dest string) error
```

# 3. 联系作者

电邮：ivfzhou@126.com
