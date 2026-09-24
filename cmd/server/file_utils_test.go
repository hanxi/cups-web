package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeNamePart(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"纯中文保留", "测试文档", "测试文档"},
		{"中文加数字", "文件1", "文件1"},
		{"中文数字连字符", "材料2-1", "材料2-1"},
		{"中英混合", "合同A", "合同A"},
		{"纯 ASCII 原样", "scan", "scan"},
		{"ASCII 带连字符", "Report-2026", "Report-2026"},
		{"空格转下划线", "my file name", "my_file_name"},
		{
			"路径危险字符全部替换",
			`a/b\c:d*e?f"g<h>i|j`,
			"a_b_c_d_e_f_g_h_i_j",
		},
		{"控制字符与 NUL 替换", "a\x00b\x1fc", "a_b_c"},
		{"点号被替换,双点无法幸存", "..", ""},
		{"路径穿越被拆散", "../../etc/passwd", "etc_passwd"},
		{"首尾下划线连字符剪除", "__hello--", "hello"},
		{"全部非法字符回落为空", "///", ""},
		{"天城文组合标记保留", "फ़ाइल", "फ़ाइल"},
		{"泰文组合元音保留", "ไฟล์", "ไฟล์"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeNamePart(tc.in); got != tc.want {
				t.Errorf("sanitizeNamePart(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeNamePartNFC(t *testing.T) {
	// "é" 的分解形式(e + U+0301)应归一化为单码点 NFC 形式后保留
	decomposed := "é"
	got := sanitizeNamePart(decomposed)
	want := "é"
	if got != want {
		t.Errorf("sanitizeNamePart(%q) = %q, want %q", decomposed, got, want)
	}
}

func TestSanitizeNamePartTruncate(t *testing.T) {
	long := strings.Repeat("扫", 200)
	got := sanitizeNamePart(long)
	if n := utf8.RuneCountInString(got); n != maxNamePartRunes {
		t.Errorf("截断后 rune 数 = %d, want %d", n, maxNamePartRunes)
	}
	// 截断后 UTF-8 字节数不应超过 ext4 单文件名 255 字节上限太多,
	// 64 rune 中文 = 192 字节,拼上时间戳与扩展名仍在限内
	if len(got) > 200 {
		t.Errorf("截断后字节数 = %d, 超出预期", len(got))
	}
}

func TestSanitizeFilenameUnicode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"报告.pdf", "报告.pdf"},
		{"扫描件.PNG", "扫描件.png"},
		{"../秘密.docx", "秘密.docx"},
		{"...pdf", "file.pdf"}, // 主体全为点号→空,回落 file
		{"", "file"},
	}
	for _, tc := range cases {
		if got := sanitizeFilename(tc.in); got != tc.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
