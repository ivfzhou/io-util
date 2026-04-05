# 一、说明

Go IO 操作工具库，提供并发安全的流转换、多路读写合并、内存管理、坐标轴标记等通用组件。

[![codecov](https://codecov.io/gh/ivfzhou/io-util/graph/badge.svg?token=QYBRAOTH5K)](https://codecov.io/gh/ivfzhou/io-util)
[![Go Reference](https://pkg.go.dev/badge/gitee.com/ivfzhou/io-util.svg)](https://pkg.go.dev/gitee.com/ivfzhou/io-util)

# 二、安装

```shell
go get gitee.com/ivfzhou/io-util@latest
```

要求 Go >= 1.22。

# 三、使用

## 3.1 WriteAt → Reader 管道转换

核心模式：创建一对写入端（`WriteAtCloser`，支持并发随机写入）和读取端（`io.ReadCloser`，顺序读出已写入数据）。写入端关闭后，读取端读完数据返回 `io.EOF`。写入端的错误会传递给读取端。

### 接口定义

```go
type WriteAtCloser interface {
    io.WriterAt
    io.Closer
    CloseByError(error) error // 带错误关闭，error 会传递给 Reader
}
```


## 3.2 多路 ReadCloser 合并

### `NewMultiReadCloserToWriterAt` — 多路并发写入 WriterAt

将多个 `io.ReadCloser` 的数据**并发**地以各自指定的偏移量写入同一个 `io.WriterAt`。

```go
func NewMultiReadCloserToWriterAt(ctx context.Context, wa io.WriterAt) (
    send func(rc io.ReadCloser, offset int64) error,
    wait func(fastExit bool) error,
)
```

- **send** — 提交一个 `rc`，数据将从 `wa` 的 `offset` 位置开始写入。所有 `rc` 在处理完毕后自动关闭。
- **wait** — 调用后等待所有数据处理完毕。`fastExit=true` 表示遇错立即返回。

### `NewMultiReadCloserToWriter` — 多路并发写入回调函数

将多个 `io.ReadCloser` **并发**读取后通过回调写入目标位置。

```go
func NewMultiReadCloserToWriter(ctx context.Context, writer func(offset int, p []byte)) (
    send func(readSize, offset int, reader io.ReadCloser),
    wait func() error,
)
```

- **send** — 提交一个 `reader`，从中读取 `readSize` 字节写入 `offset` 位置。
- **wait** — 等待全部读取完成。

### `NewMultiReadCloserToReader` — 多路顺序合并为单一 Reader

将多个 `io.ReadCloser` **顺序**合并为一个 `io.Reader`，读完一个自动切换到下一个。

```go
func NewMultiReadCloserToReader(ctx context.Context, rc ...io.ReadCloser) (
    r io.Reader,
    add func(rc io.ReadCloser) error,
    endAdd func(),
)
```

- **r** — 合并后的读取流。
- **add** — 动态添加新的 `rc`。发生错误时停止添加，已添加的 `rc` 全部关闭。
- **endAdd** — 声明不再添加新 `rc`，所有数据读完后 `r` 返回 `io.EOF`（务必调用）。

---

## 3.3 Reader → WriterAt 流式拷贝

```go
func CopyReaderToWriterAt(r io.Reader, w io.WriterAt, offset int64, nonBuffer bool) (written int64, err error)
```

将 `r` 的数据流式写入 `w` 的指定偏移位置，直到 `r` 返回 `io.EOF`。`nonBuffer=true` 每次分配新内存；`false` 复用缓冲区并按需扩容。
