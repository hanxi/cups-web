package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestParseFontMetadata_EmbeddedNotoSansSC(t *testing.T) {
	if len(notoSansSCFont) == 0 {
		t.Skip("embedded NotoSansSC font is empty")
	}

	tmpDir := t.TempDir()
	// 刻意命名为 1.ttf，模拟用户上传数字命名文件
	fakeFilePath := filepath.Join(tmpDir, "1.ttf")
	if err := os.WriteFile(fakeFilePath, notoSansSCFont, 0644); err != nil {
		t.Fatalf("failed to write test font file: %v", err)
	}

	meta, err := ParseFontMetadataFromFile(fakeFilePath)
	if err != nil {
		t.Fatalf("ParseFontMetadataFromFile failed: %v", err)
	}

	// 真实字体名称应该包含 Noto Sans SC，而不是单纯的文件名 "1"
	if !strings.Contains(meta.DisplayName, "Noto") && !strings.Contains(meta.Family, "Noto") {
		t.Errorf("expected font name to contain 'Noto', got DisplayName=%q, Family=%q", meta.DisplayName, meta.Family)
	}
	if meta.Format != "TTF" && meta.Format != "OTF" {
		t.Errorf("expected format TTF or OTF, got %q", meta.Format)
	}
}

func TestParseFontMetadata_SyntheticTTF(t *testing.T) {
	// 构造一个最小合法的 TrueType 文件，包含标准的 'name' 表
	// 包含中文名 "仿宋" (zh-CN) 和英文名 "FangSong" (en-US)
	ttfData := createMockTTFWithNameTable("仿宋", "FangSong", "Regular")

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "custom_123.ttf")
	if err := os.WriteFile(filePath, ttfData, 0644); err != nil {
		t.Fatalf("failed to write mock ttf: %v", err)
	}

	meta, err := ParseFontMetadataFromFile(filePath)
	if err != nil {
		t.Fatalf("ParseFontMetadataFromFile failed: %v", err)
	}

	expectedDisplay := "仿宋 (FangSong)"
	if meta.DisplayName != expectedDisplay {
		t.Errorf("expected DisplayName %q, got %q", expectedDisplay, meta.DisplayName)
	}
	if meta.Family != "仿宋" && meta.Family != "FangSong" {
		t.Errorf("expected Family to be '仿宋' or 'FangSong', got %q", meta.Family)
	}
	if meta.Subfamily != "Regular" {
		t.Errorf("expected Subfamily 'Regular', got %q", meta.Subfamily)
	}
	if meta.Format != "TTF" {
		t.Errorf("expected Format 'TTF', got %q", meta.Format)
	}
}

func TestParseFontMetadata_FallbackOnInvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	brokenFile := filepath.Join(tmpDir, "broken.ttf")
	if err := os.WriteFile(brokenFile, []byte("not a font file"), 0644); err != nil {
		t.Fatalf("failed to write broken file: %v", err)
	}

	meta, _ := ParseFontMetadataFromFile(brokenFile)
	if meta.DisplayName != "broken" {
		t.Errorf("expected fallback display name 'broken', got %q", meta.DisplayName)
	}
	if meta.Format != "TTF" {
		t.Errorf("expected Format 'TTF', got %q", meta.Format)
	}
}

// helper: 构造包含 name 表的 mock TrueType 二进制数据
func createMockTTFWithNameTable(familyZh, familyEn, subfamily string) []byte {
	var stringData bytes.Buffer
	type record struct {
		platformID uint16
		encodingID uint16
		languageID uint16
		nameID     uint16
		strBytes   []byte
	}

	toUTF16BE := func(s string) []byte {
		runes := utf16.Encode([]rune(s))
		buf := make([]byte, len(runes)*2)
		for i, r := range runes {
			binary.BigEndian.PutUint16(buf[i*2:], r)
		}
		return buf
	}

	records := []record{
		// nameID 1: Family zh-CN
		{platformID: 3, encodingID: 1, languageID: 0x0804, nameID: 1, strBytes: toUTF16BE(familyZh)},
		// nameID 1: Family en-US
		{platformID: 3, encodingID: 1, languageID: 0x0409, nameID: 1, strBytes: toUTF16BE(familyEn)},
		// nameID 2: Subfamily en-US
		{platformID: 3, encodingID: 1, languageID: 0x0409, nameID: 2, strBytes: toUTF16BE(subfamily)},
	}

	type encodedRecord struct {
		platformID uint16
		encodingID uint16
		languageID uint16
		nameID     uint16
		length     uint16
		offset     uint16
	}

	var encRecords []encodedRecord
	for _, rec := range records {
		offset := stringData.Len()
		stringData.Write(rec.strBytes)
		encRecords = append(encRecords, encodedRecord{
			platformID: rec.platformID,
			encodingID: rec.encodingID,
			languageID: rec.languageID,
			nameID:     rec.nameID,
			length:     uint16(len(rec.strBytes)),
			offset:     uint16(offset),
		})
	}

	// 组装 name 表
	var nameTable bytes.Buffer
	binary.Write(&nameTable, binary.BigEndian, uint16(0)) // format = 0
	binary.Write(&nameTable, binary.BigEndian, uint16(len(encRecords)))
	stringOffset := uint16(6 + len(encRecords)*12)
	binary.Write(&nameTable, binary.BigEndian, stringOffset)
	for _, er := range encRecords {
		binary.Write(&nameTable, binary.BigEndian, er.platformID)
		binary.Write(&nameTable, binary.BigEndian, er.encodingID)
		binary.Write(&nameTable, binary.BigEndian, er.languageID)
		binary.Write(&nameTable, binary.BigEndian, er.nameID)
		binary.Write(&nameTable, binary.BigEndian, er.length)
		binary.Write(&nameTable, binary.BigEndian, er.offset)
	}
	nameTable.Write(stringData.Bytes())

	// 组装整个 TTF（Header + 1 个 table directory + name table bytes）
	var ttf bytes.Buffer
	binary.Write(&ttf, binary.BigEndian, uint32(0x00010000)) // sfntVersion
	binary.Write(&ttf, binary.BigEndian, uint16(1))          // numTables = 1
	binary.Write(&ttf, binary.BigEndian, uint16(16))         // searchRange
	binary.Write(&ttf, binary.BigEndian, uint16(0))          // entrySelector
	binary.Write(&ttf, binary.BigEndian, uint16(0))          // rangeShift

	tableOffset := uint32(12 + 16) // header (12) + 1 table record (16)
	tableLen := uint32(nameTable.Len())

	// Table Directory Record
	ttf.WriteString("name")
	binary.Write(&ttf, binary.BigEndian, uint32(0)) // checksum
	binary.Write(&ttf, binary.BigEndian, tableOffset)
	binary.Write(&ttf, binary.BigEndian, tableLen)

	ttf.Write(nameTable.Bytes())
	return ttf.Bytes()
}
