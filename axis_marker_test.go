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
	"testing"

	"gitee.com/ivfzhou/io-util"
)

func TestAxisMarker(t *testing.T) {
	am := &io_util.AxisMarker{}
	am.Mark(1, 1)
	am.Mark(3, 1)
	am.Mark(5, 1)
	am.Mark(7, 1)
	am.Mark(9, 1)
	am.Mark(11, 1)
	am.Mark(13, 1)
	if l := am.GetMaxMarkLine(1); l != 1 {
		t.Error("axis_marker: failed to match, expected 1 but give", l)
	}
	am.Mark(2, 5)
	if l := am.GetMaxMarkLine(2); l != 6 {
		t.Error("axis_marker: failed to match, expected 6 but give", l)
	}
	if l := am.GetMaxMarkLine(1); l != 7 {
		t.Error("axis_marker: failed to match, expected 7 but give", l)
	}
	am.Mark(3, 9)
	if l := am.GetMaxMarkLine(1); l != 11 {
		t.Error("axis_marker: failed to match, expected 11 but give", l)
	}
	if len(am.String()) <= 0 {
		t.Error("axis_marker: string can not empty")
	}
}
