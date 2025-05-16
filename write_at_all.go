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

import "io"

// WriteAtAll 将数据全部写入 wa。
func WriteAtAll(wa io.WriterAt, offset int64, p []byte) (written int64, err error) {
	n := 0
	for len(p) > 0 {
		n, err = wa.WriteAt(p, offset)
		written += int64(n)
		offset += int64(n)
		if err != nil {
			return written, err
		}
		p = p[min(n, len(p)):]
	}
	return
}
